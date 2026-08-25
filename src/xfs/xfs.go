package xfsdriver

// #include "./xfs.h"
import "C"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"
)

const BlockDeviceName = "__control-device"

// Quota defines the limit and usage fields for an XFS project.
// Fields suffixed with Used are read-only (populated by Get operations).
type Quota struct {
	BlockHardLimit uint64
	BlockSoftLimit uint64
	BlocksUsed     uint64

	InodeHardLimit uint64
	InodeSoftLimit uint64
	InodesUsed     uint64
}

// xfs_quota -x -c 'project -s -p ProjectDirectory ProjectID' XFSMountPoint
func SetProjectIdViaCmd(XFSMountPoint string, projectId uint32, directory string) error {
	if _, err := os.Lstat(directory); err != nil {
		return fmt.Errorf("xfs_quota: directory %q not found: %w", directory, err)
	}
	projectCommand := fmt.Sprintf("project -s -p %s %d", directory, projectId)
	_, err := exec.Command("xfs_quota", "-x", "-c", projectCommand, XFSMountPoint).Output()
	return err
}

// xfs_quota -x -c 'project -C -p <project_path> <project-id>' <xfs_mount_path>
func UnSetProjectIdViaCmd(XFSMountPoint string, projectId uint32, directory string) error {
	if _, err := os.Lstat(directory); err != nil {
		// Directory already gone — quota was removed via quotactl; treat as success.
		return nil
	}
	projectCommand := fmt.Sprintf("project -C -p %s %d", directory, projectId)
	_, err := exec.Command("xfs_quota", "-x", "-c", projectCommand, XFSMountPoint).Output()
	return err
}

// SetProjectQuota applies quota limits for a project on an XFS mount.
// Zero values indicate no limit.
func SetProjectQuota(XFSMountPoint string, projectId uint32, q *Quota) (err error) {
	if XFSMountPoint == "" {
		return errors.New("XFS mount point must be set")
	}
	blockDevice := filepath.Join(XFSMountPoint, BlockDeviceName)
	if _, err := os.Stat(blockDevice); os.IsNotExist(err) {
		if err = MakeBackingFsDev(XFSMountPoint, BlockDeviceName); err != nil {
			return err
		}
	}

	var (
		blockDeviceString = C.CString(blockDevice)
		quota             = &C.struct_xfs_quota{
			block_hard_limit: C.__u64(q.BlockHardLimit),
			block_soft_limit: C.__u64(q.BlockSoftLimit),
			inode_hard_limit: C.__u64(q.InodeHardLimit),
			inode_soft_limit: C.__u64(q.InodeSoftLimit),
		}
	)
	defer C.free(unsafe.Pointer(blockDeviceString))

	ret, err := C.xfs_set_project_quota(blockDeviceString, C.__u32(projectId), quota)
	if ret == -1 {
		return errors.New(err.Error() +
			"failed to set project quota " +
			"prj=" + fmt.Sprint(projectId) + " dev=" + blockDevice +
			" quota-size=" + fmt.Sprint(q.BlockHardLimit) +
			" quota-inodes=" + fmt.Sprint(q.InodeHardLimit))
	}
	return nil
}

// UnSetProjectQuota removes all quota limits from a project (sets all to 0).
func UnSetProjectQuota(XFSMountPoint string, projectId uint32) (err error) {
	if XFSMountPoint == "" {
		return errors.New("XFS mount point must be set")
	}
	blockDevice := filepath.Join(XFSMountPoint, BlockDeviceName)
	if _, err := os.Stat(blockDevice); os.IsNotExist(err) {
		if err = MakeBackingFsDev(XFSMountPoint, BlockDeviceName); err != nil {
			return err
		}
	}

	var (
		blockDeviceString = C.CString(blockDevice)
		quota             = &C.struct_xfs_quota{} // all zeros = no limits
	)
	defer C.free(unsafe.Pointer(blockDeviceString))

	ret, err := C.xfs_set_project_quota(blockDeviceString, C.__u32(projectId), quota)
	if ret == -1 {
		return errors.New(err.Error() +
			"failed to unset project quota " +
			"prj=" + fmt.Sprint(projectId) + " dev=" + blockDevice)
	}
	return nil
}

// IsQuotaEnabled checks whether project quota accounting and enforcement are
// active on the filesystem at XFSMountPoint.
func IsQuotaEnabled(XFSMountPoint string) (isEnabled bool, err error) {
	if XFSMountPoint == "" {
		return false, errors.New("XFS mount point must be set")
	}
	blockDevice := filepath.Join(XFSMountPoint, BlockDeviceName)
	if _, err := os.Stat(blockDevice); os.IsNotExist(err) {
		if err = MakeBackingFsDev(XFSMountPoint, BlockDeviceName); err != nil {
			return false, err
		}
	}

	var blockDeviceString = C.CString(blockDevice)
	defer C.free(unsafe.Pointer(blockDeviceString))

	ret, err := C.xfs_is_quota_enabled(blockDeviceString)
	switch ret {
	case -1:
		return false, errors.New(err.Error() +
			"failed to check whether quota is enabled for dev " + blockDevice)
	case 0:
		isEnabled = true
	}
	return
}

// GetProjectQuota retrieves the current quota limits and usage for a project.
func GetProjectQuota(XFSMountPoint string, projectId uint32) (q *Quota, err error) {
	if XFSMountPoint == "" {
		return nil, errors.New("XFS mount point must be set")
	}

	blockDevice := filepath.Join(XFSMountPoint, BlockDeviceName)
	if _, err := os.Stat(blockDevice); os.IsNotExist(err) {
		if err = MakeBackingFsDev(XFSMountPoint, BlockDeviceName); err != nil {
			return nil, err
		}
	}

	var (
		blockDeviceString = C.CString(blockDevice)
		quota             = new(C.struct_xfs_quota)
	)
	defer C.free(unsafe.Pointer(blockDeviceString))

	ret, err := C.xfs_get_project_quota(blockDeviceString, C.__u32(projectId), quota)
	if ret == -1 {
		return nil, errors.New(err.Error() +
			"failed to retrieve project quota - prj=" + fmt.Sprint(projectId) + " dev=" + blockDevice)
	}

	q = &Quota{
		BlockHardLimit: uint64(quota.block_hard_limit),
		BlockSoftLimit: uint64(quota.block_soft_limit),
		InodeHardLimit: uint64(quota.inode_hard_limit),
		InodeSoftLimit: uint64(quota.inode_soft_limit),
		BlocksUsed:     uint64(quota.blocks_used),
		InodesUsed:     uint64(quota.inodes_used),
	}
	return
}

// GetProjectId retrieves the XFS project ID set on a directory via its extended attribute.
func GetProjectId(directory string) (projectId uint32, err error) {
	var directoryString = C.CString(directory)
	defer C.free(unsafe.Pointer(directoryString))

	ret, err := C.xfs_get_project_id(directoryString)
	if ret == -1 {
		return 0, errors.New(err.Error() +
			"failed to get project-id from directory ::" + directory)
	}
	return uint32(ret), nil
}

// SetProjectId sets the XFS project ID and PROJINHERIT flag on a directory.
func SetProjectId(directory string, projectId uint32) (err error) {
	var directoryString = C.CString(directory)
	defer C.free(unsafe.Pointer(directoryString))

	ret, err := C.xfs_set_project_id(directoryString, C.__u32(projectId))
	if ret == -1 {
		return errors.New(err.Error() +
			"failed to set project-id ::" + fmt.Sprint(projectId) + " to directory ::" + directory)
	}
	return nil
}

// MakeBackingFsDev creates a control block device under root/file used by quotactl.
func MakeBackingFsDev(root, file string) (err error) {
	if root == "" || file == "" {
		return errors.New("root and file must be provided")
	}

	var (
		rootString = C.CString(root)
		fileString = C.CString(file)
	)
	defer C.free(unsafe.Pointer(rootString))
	defer C.free(unsafe.Pointer(fileString))

	ret, err := C.xfs_create_fs_block_dev(rootString, fileString)
	if ret == -1 {
		return errors.New(err.Error() +
			"failed to create fs block device " + root + "/" + file)
	}
	return nil
}
