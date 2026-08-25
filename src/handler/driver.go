package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"volume-plugin/store"
	"volume-plugin/utils"
	xfsdriver "volume-plugin/xfs"

	"github.com/docker/go-plugins-helpers/volume"
	log "github.com/sirupsen/logrus"
)

type xfsVol struct {
	XFSdriverconfig *xfsdriver.XFSDriver
	Bean            *storeAccess
	Config          *utils.ConfigVariables
	Context         context.Context
}

func IntializeXFSDriver(bean *storeAccess, config *utils.ConfigVariables) *xfsVol {
	return &xfsVol{
		Context:         context.Background(),
		XFSdriverconfig: &xfsdriver.XFSDriver{M: &sync.Mutex{}},
		Bean:            bean,
		Config:          config,
	}
}

// Create sets up XFS project quota for a new volume and persists it to state.
func (d *xfsVol) Create(r *volume.CreateRequest) (err error) {
	d.XFSdriverconfig.M.Lock()
	defer d.XFSdriverconfig.M.Unlock()

	options, err := utils.IntCreateVar(r, d.Config)
	if err != nil {
		return err
	}

	available, err := d.Bean.store.CheckIfMountPointAvailable(options.OptedMountPoint)
	if !available {
		log.Info("mountpoint already in use: ", options.OptedMountPoint)
		return errors.New("given path is already in use")
	}

	projectID, err := utils.GetCurrentProjectID(d.Config, d.Bean.store)
	if err != nil {
		log.Error("failed to get current project ID: ", err)
		return err
	}
	if err = d.Bean.store.SetPropertyProjectID(strconv.Itoa(projectID)); err != nil {
		log.Error("failed to update project ID counter: ", err)
		return err
	}

	proDetails := store.XFSVolDetails{
		ProjectID:      projectID,
		VolumeName:     r.Name,
		VolumePath:     options.OptedMountPoint,
		CreatedAt:      time.Now().Format(time.RFC3339),
		BlockHardLimit: strconv.Itoa(int(options.GetOptedBlockHardLimit())),
		BlockSoftLimit: strconv.Itoa(int(options.GetOptedBlockSoftLimit())),
		InodeHardLimit: strconv.Itoa(int(options.GetOptedInodeHardLimit())),
		InodeSoftLimit: strconv.Itoa(int(options.GetOptedInodeSoftLimit())),
		ReadBPS:        options.ReadBPS,
		WriteBPS:       options.WriteBPS,
		ReadIOPS:       options.ReadIOPS,
		WriteIOPS:      options.WriteIOPS,
	}

	// InsertToDB: record an already-configured project without applying quota again.
	if _, ok := r.Options["InsertToDB"]; ok {
		optedID := r.Options["ProjectID"]
		id, err := xfsdriver.GetProjectId(options.OptedMountPoint)
		if err != nil {
			return err
		}
		if strconv.Itoa(int(id)) != optedID {
			return errors.New("project ID does not exist")
		}
		quota, err := xfsdriver.GetProjectQuota(d.Config.DefaultXFSMountPoint, uint32(id))
		if err != nil {
			log.Error("failed to get project quota: ", err)
			return err
		}
		proDetails.ProjectID = int(id)
		proDetails.BlockHardLimit = strconv.Itoa(int(quota.BlockHardLimit))
		proDetails.BlockSoftLimit = strconv.Itoa(int(quota.BlockSoftLimit))
		proDetails.InodeHardLimit = strconv.Itoa(int(quota.InodeHardLimit))
		proDetails.InodeSoftLimit = strconv.Itoa(int(quota.InodeSoftLimit))
		_, err = d.addVolumeAndProjectDetails(proDetails)
		return err
	}

	if err = utils.ProjectQuotaCmd(d.Config, options, proDetails); err != nil {
		return err
	}
	if err = xfsdriver.SetProjectIdViaCmd(d.Config.DefaultXFSMountPoint, uint32(proDetails.ProjectID), options.OptedMountPoint); err != nil {
		return err
	}

	if _, err = d.addVolumeAndProjectDetails(proDetails); err != nil {
		log.Error("failed to persist volume — rolling back quota: ", err)
		xfsdriver.UnSetProjectIdViaCmd(d.Config.DefaultXFSMountPoint, uint32(proDetails.ProjectID), options.OptedMountPoint)
		xfsdriver.UnSetProjectQuota(d.Config.DefaultXFSMountPoint, uint32(proDetails.ProjectID))
		if rmErr := utils.RmEmptyDir(options.OptedMountPoint); rmErr != nil {
			log.Error("failed to remove directory on rollback: ", rmErr)
		}
		return err
	}

	log.Info("volume created: ", r.Name)
	return nil
}

// List returns all volumes with their current quota status.
func (d *xfsVol) List() (*volume.ListResponse, error) {
	_, err := d.Bean.store.GetDockerVolumeCount()
	if err != nil {
		return nil, err
	}

	all, err := d.Bean.store.GetALLVolumes()
	if err != nil {
		log.Error("failed to list volumes: ", err)
		return nil, err
	}

	volumes := make([]*volume.Volume, 0, len(all))
	for _, v := range all {
		quota, err := xfsdriver.GetProjectQuota(d.Config.DefaultXFSMountPoint, uint32(v.ProjectID))
		if err != nil {
			log.Error("failed to get project quota for ", v.VolumeName, ": ", err)
			return nil, err
		}
		volumes = append(volumes, &volume.Volume{
			Name:       v.VolumeName,
			Mountpoint: v.VolumePath,
			CreatedAt:  v.CreatedAt,
			Status: map[string]interface{}{
				"XFSProjectID":   v.ProjectID,
				"BlockHardLimit": quota.BlockHardLimit,
				"BlockSoftLimit": quota.BlockSoftLimit,
				"BlocksUsed":     quota.BlocksUsed,
				"InodeHardLimit": quota.InodeHardLimit,
				"InodeSoftLimit": quota.InodeSoftLimit,
				"InodesUsed":     quota.InodesUsed,
				"ReadBPS":        v.ReadBPS,
				"WriteBPS":       v.WriteBPS,
				"ReadIOPS":       v.ReadIOPS,
				"WriteIOPS":      v.WriteIOPS,
			},
		})
	}
	return &volume.ListResponse{Volumes: volumes}, nil
}

// Get returns the details and current quota status for a single volume.
func (d *xfsVol) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.XFSdriverconfig.M.Lock()
	defer d.XFSdriverconfig.M.Unlock()

	volDetails, projDetails, err := d.Bean.store.GetALLDetailsByName(r.Name)
	if err != nil {
		return &volume.GetResponse{}, err
	}

	quota, err := xfsdriver.GetProjectQuota(d.Config.DefaultXFSMountPoint, uint32(projDetails.ProjectID))
	if err != nil {
		log.Error("failed to get project quota: ", err)
		return nil, err
	}

	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name:       r.Name,
			Mountpoint: volDetails.VolumePath,
			CreatedAt:  projDetails.CreatedAt,
			Status: map[string]interface{}{
				"XFSProjectID":   projDetails.ProjectID,
				"BlockHardLimit": quota.BlockHardLimit,
				"BlockSoftLimit": quota.BlockSoftLimit,
				"BlocksUsed":     quota.BlocksUsed,
				"InodeHardLimit": quota.InodeHardLimit,
				"InodeSoftLimit": quota.InodeSoftLimit,
				"InodesUsed":     quota.InodesUsed,
				"ReadBPS":        projDetails.ReadBPS,
				"WriteBPS":       projDetails.WriteBPS,
				"ReadIOPS":       projDetails.ReadIOPS,
				"WriteIOPS":      projDetails.WriteIOPS,
			},
		},
	}, nil
}

// Remove clears the XFS quota, removes the state entry, and deletes the directory.
func (d *xfsVol) Remove(r *volume.RemoveRequest) error {
	d.XFSdriverconfig.M.Lock()
	defer d.XFSdriverconfig.M.Unlock()

	volDetails, projDetails, err := d.Bean.store.GetALLDetailsByName(r.Name)
	if err != nil {
		return err
	}

	if err = xfsdriver.UnSetProjectIdViaCmd(d.Config.DefaultXFSMountPoint, uint32(projDetails.ProjectID), volDetails.VolumePath); err != nil {
		return err
	}
	if err = xfsdriver.UnSetProjectQuota(d.Config.DefaultXFSMountPoint, uint32(projDetails.ProjectID)); err != nil {
		log.Error("failed to unset project quota: ", err)
		return err
	}

	_, err = d.Bean.store.RemoveDockerVolumeByName(r.Name)
	if err != nil {
		log.Error("failed to remove volume from state: ", err)
		return errors.New("error while saving in state file")
	}

	if err = utils.RmEmptyDir(volDetails.VolumePath); err != nil {
		log.Error("failed to remove directory: ", err)
	}
	return nil
}

// Path returns the mountpoint for a volume.
func (d *xfsVol) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	path, err := d.Bean.store.GetDockerVolumePathByName(r.Name)
	if err != nil {
		log.Error("failed to get volume path: ", err)
		return &volume.PathResponse{}, errors.New("volume path does not exist")
	}
	return &volume.PathResponse{Mountpoint: path}, nil
}

// Mount returns the mountpoint path and applies any configured cgroup blkio
// throttle limits for the container mounting this volume.
func (d *xfsVol) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	_, proj, err := d.Bean.store.GetALLDetailsByName(r.Name)
	if err != nil {
		log.Error("mount failed — volume not found: ", err)
		return nil, err
	}
	path, err := d.Bean.store.GetDockerVolumePathByName(r.Name)
	if err != nil {
		return nil, err
	}
	if err = utils.ApplyCgroupBlkioLimits(r.ID, d.Config.DefaultXFSMountPoint,
		proj.ReadBPS, proj.WriteBPS, proj.ReadIOPS, proj.WriteIOPS); err != nil {
		// Non-fatal: volume still works without cgroup limits.
		log.Warn("cgroup blkio limits not applied: ", err)
	}
	return &volume.MountResponse{Mountpoint: path}, nil
}

// Unmount clears any cgroup blkio limits set at mount time.
// The actual unmount is handled by Docker.
func (d *xfsVol) Unmount(r *volume.UnmountRequest) error {
	_, proj, err := d.Bean.store.GetALLDetailsByName(r.Name)
	if err != nil {
		log.Error("unmount failed — volume not found: ", err)
		return errors.New("volume does not exist for unmount")
	}
	utils.RemoveCgroupBlkioLimits(r.ID, d.Config.DefaultXFSMountPoint,
		proj.ReadBPS, proj.WriteBPS, proj.ReadIOPS, proj.WriteIOPS)
	return nil
}

// Capabilities reports that this is a local-scoped driver.
func (d *xfsVol) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

// addVolumeAndProjectDetails writes the volume and its XFS project record to state.
func (d *xfsVol) addVolumeAndProjectDetails(proDetails store.XFSVolDetails) (bool, error) {
	if err := d.Bean.store.AddDockerVolume(proDetails.VolumeName, proDetails.VolumePath); err != nil {
		log.Error("failed to add docker volume: ", err)
		return false, err
	}
	id, err := d.Bean.store.GetDockerVolumeIDByName(proDetails.VolumeName)
	if id == -1 || err != nil {
		log.Error("failed to fetch volume ID after insert: ", err)
		return false, fmt.Errorf("volume %q not found after insert", proDetails.VolumeName)
	}
	if err = d.Bean.store.AddXFSProject(proDetails, id); err != nil {
		log.Error("failed to add XFS project: ", err)
		return false, err
	}
	return true, nil
}
