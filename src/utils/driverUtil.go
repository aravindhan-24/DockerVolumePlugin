package utils

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"volume-plugin/store"
	xfsdriver "volume-plugin/xfs"

	log "github.com/sirupsen/logrus"

	"github.com/docker/go-plugins-helpers/volume"
)

type CreateProjectOptions struct {
	OptedMountPoint                  string
	OptedXFSDiskMountPoint           string
	OptedHumanReadableBlockHardLimit uint64
	OptedHumanReadableBlockSoftLimit uint64
	OptedHumanReadableInodeHardLimit uint64
	OptedHumanReadableInodeSoftLimit uint64
	// Cgroup blkio throttle limits (0 = no limit).
	ReadBPS   uint64
	WriteBPS  uint64
	ReadIOPS  uint64
	WriteIOPS uint64
}

func (o *CreateProjectOptions) GetOptedBlockHardLimit() uint64 {
	return o.OptedHumanReadableBlockHardLimit
}

func (o *CreateProjectOptions) GetOptedBlockSoftLimit() uint64 {
	return o.OptedHumanReadableBlockSoftLimit
}

func (o *CreateProjectOptions) GetOptedInodeHardLimit() uint64 {
	return o.OptedHumanReadableInodeHardLimit
}

func (o *CreateProjectOptions) GetOptedInodeSoftLimit() uint64 {
	return o.OptedHumanReadableInodeSoftLimit
}

// changeOwnerAndGroup chowns path and its ancestors up to (and including) stopAt.
// stopAt should be the XFS disk mount point so the walk never reaches system directories.
func changeOwnerAndGroup(path string, uid, gid int, stopAt string) error {
	for {
		if err := os.Chown(path, uid, gid); err != nil {
			return err
		}
		if path == stopAt {
			break
		}
		newPath := filepath.Dir(path)
		if newPath == path {
			break
		}
		path = newPath
	}
	return nil
}

// IntCreateVar parses the create request options and validates the mount point.
func IntCreateVar(r *volume.CreateRequest, cn *ConfigVariables) (*CreateProjectOptions, error) {
	resultantMap, err := ParseOptionsAndConfig(r.Options, cn)
	if err != nil {
		return nil, err
	}
	o := &CreateProjectOptions{
		OptedMountPoint:                  r.Options["MountPoint"],
		OptedXFSDiskMountPoint:           r.Options["XFSDiskMountPoint"],
		OptedHumanReadableBlockHardLimit: resultantMap["BlockHardLimit"],
		OptedHumanReadableBlockSoftLimit: resultantMap["BlockSoftLimit"],
		OptedHumanReadableInodeHardLimit: resultantMap["BlockInodeHardLimit"],
		OptedHumanReadableInodeSoftLimit: resultantMap["BlockInodeSoftLimit"],
	}

	if o.OptedMountPoint == "" {
		log.Warn("MountPoint option is missing")
		return nil, errors.New("missing required parameter: MountPoint")
	}
	if !strings.Contains(o.OptedMountPoint, o.OptedXFSDiskMountPoint) {
		log.Warn("opted mountpoint is outside the XFS disk mountpoint")
		return nil, errors.New("opted mountpoint is not within the XFS disk mountpoint")
	}

	if err = os.MkdirAll(o.OptedMountPoint, 0755); err != nil {
		return nil, err
	}
	if _, err = os.Lstat(o.OptedMountPoint); err != nil {
		log.Warn("mount point path does not exist: ", err)
		return nil, errors.New("mountpoint does not exist")
	}

	group, err := user.Lookup(cn.OwnerOfMountPoint)
	if err != nil {
		return nil, fmt.Errorf("error looking up user %q: user not found", cn.OwnerOfMountPoint)
	}
	uid, err := strconv.Atoi(group.Uid)
	if err != nil {
		return nil, fmt.Errorf("non-numeric UID for user %q: %w", cn.OwnerOfMountPoint, err)
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return nil, fmt.Errorf("non-numeric GID for user %q: %w", cn.OwnerOfMountPoint, err)
	}
	if err = changeOwnerAndGroup(o.OptedMountPoint, uid, gid, o.OptedXFSDiskMountPoint); err != nil {
		return nil, err
	}

	// Parse cgroup blkio limits (BPS as byte-size strings, IOPS as plain integers).
	parseBPS := func(key, defaultVal string) (uint64, error) {
		if v, ok := r.Options[key]; ok && v != "" {
			return getByteSize(v)
		}
		if defaultVal == "" || defaultVal == "0" {
			return 0, nil
		}
		return getByteSize(defaultVal)
	}
	parseIOPS := func(key, defaultVal string) (uint64, error) {
		if v, ok := r.Options[key]; ok && v != "" {
			return strconv.ParseUint(v, 10, 64)
		}
		if defaultVal == "" || defaultVal == "0" {
			return 0, nil
		}
		return strconv.ParseUint(defaultVal, 10, 64)
	}
	if o.ReadBPS, err = parseBPS("ReadBPS", cn.DefaultReadBPS); err != nil {
		return nil, fmt.Errorf("invalid ReadBPS: %w", err)
	}
	if o.WriteBPS, err = parseBPS("WriteBPS", cn.DefaultWriteBPS); err != nil {
		return nil, fmt.Errorf("invalid WriteBPS: %w", err)
	}
	if o.ReadIOPS, err = parseIOPS("ReadIOPS", cn.DefaultReadIOPS); err != nil {
		return nil, fmt.Errorf("invalid ReadIOPS: %w", err)
	}
	if o.WriteIOPS, err = parseIOPS("WriteIOPS", cn.DefaultWriteIOPS); err != nil {
		return nil, fmt.Errorf("invalid WriteIOPS: %w", err)
	}

	// Cap soft limits to hard limits.
	if o.OptedHumanReadableBlockSoftLimit > o.OptedHumanReadableBlockHardLimit {
		o.OptedHumanReadableBlockSoftLimit = o.OptedHumanReadableBlockHardLimit
	}
	if o.OptedHumanReadableInodeSoftLimit > o.OptedHumanReadableInodeHardLimit {
		o.OptedHumanReadableInodeSoftLimit = o.OptedHumanReadableInodeHardLimit
	}
	return o, nil
}

func checkQuota(quota *xfsdriver.Quota) bool {
	return checkBlock(quota) || checkInode(quota)
}
func checkBlock(quota *xfsdriver.Quota) bool {
	return quota.BlockHardLimit != 0 || quota.BlockSoftLimit != 0 || quota.BlocksUsed != 0
}
func checkInode(quota *xfsdriver.Quota) bool {
	return quota.InodeHardLimit != 0 || quota.InodeSoftLimit != 0 || quota.InodesUsed != 0
}

// GetCurrentProjectID returns the next available XFS project ID that is not
// already in use on disk or in the state store.
func GetCurrentProjectID(C *ConfigVariables, db store.Datastore) (int, error) {
	if err := db.InitializePropertyProjectID(); err != nil {
		log.Info(err)
		return 0, err
	}
	currentID, err := db.GetPropertyProjectIDValue()
	if err != nil {
		log.Error("unable to get current project ID")
		return 0, err
	}
	if currentID == -1 {
		currentID = 1
	}

	for {
		quota, err := xfsdriver.GetProjectQuota(C.DefaultXFSMountPoint, uint32(currentID))
		if err != nil {
			if strings.Contains(err.Error(), "failed to retrieve project quota") {
				break
			}
			return currentID, err
		}
		if checkQuota(quota) {
			currentID++
		} else if idExists, _ := db.CheckIfXFSProjectIDExists(currentID); idExists {
			currentID++
		} else {
			break
		}
		// XFS project IDs are 32-bit; wrap around before the max.
		if currentID >= 4294967295 {
			currentID = 1
		}
	}
	return currentID, nil
}

// ProjectQuotaCmd applies the XFS project ID and quota limits for a new volume.
func ProjectQuotaCmd(C *ConfigVariables, options *CreateProjectOptions, ProDetails store.XFSVolDetails) error {
	if err := xfsdriver.SetProjectId(options.OptedMountPoint, uint32(ProDetails.ProjectID)); err != nil {
		return err
	}
	return xfsdriver.SetProjectQuota(C.DefaultXFSMountPoint, uint32(ProDetails.ProjectID), &xfsdriver.Quota{
		BlockHardLimit: options.GetOptedBlockHardLimit(),
		BlockSoftLimit: options.GetOptedBlockSoftLimit(),
		InodeHardLimit: options.GetOptedInodeHardLimit(),
		InodeSoftLimit: options.GetOptedInodeSoftLimit(),
	})
}
