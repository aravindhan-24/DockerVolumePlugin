package utils

import (
	"fmt"
	"os"
	"syscall"

	log "github.com/sirupsen/logrus"
)

// containerCgroupDir finds the blkio cgroup directory for the container with
// the given ID. Docker may use cgroupfs or systemd as its cgroup driver, and
// the host may run cgroup v1 or v2. We probe all four candidate paths and
// return the first one that exists on disk, together with a boolean that is
// true when the directory belongs to a cgroup v2 hierarchy (io.max interface).
func containerCgroupDir(containerID string) (dir string, v2 bool, err error) {
	type candidate struct {
		path string
		v2   bool
	}
	candidates := []candidate{
		// cgroupv2 — systemd cgroup driver
		{fmt.Sprintf("/sys/fs/cgroup/system.slice/docker-%s.scope", containerID), true},
		// cgroupv2 — cgroupfs cgroup driver
		{fmt.Sprintf("/sys/fs/cgroup/docker/%s", containerID), true},
		// cgroupv1 — systemd cgroup driver
		{fmt.Sprintf("/sys/fs/cgroup/blkio/system.slice/docker-%s.scope", containerID), false},
		// cgroupv1 — cgroupfs cgroup driver
		{fmt.Sprintf("/sys/fs/cgroup/blkio/docker/%s", containerID), false},
	}
	for _, c := range candidates {
		if _, err2 := os.Stat(c.path); err2 == nil {
			return c.path, c.v2, nil
		}
	}
	return "", false, fmt.Errorf("cgroup dir not found for container %.12s", containerID)
}

// devMajorMinor returns the major and minor numbers of the block device that
// backs the filesystem at path (e.g. the XFS loop mount point).
func devMajorMinor(path string) (major, minor uint32, err error) {
	var st syscall.Stat_t
	if err = syscall.Stat(path, &st); err != nil {
		return 0, 0, fmt.Errorf("stat %q: %w", path, err)
	}
	// Standard Linux device-number encoding (makedev / major / minor).
	major = uint32((st.Dev>>8)&0xfff) | uint32((st.Dev>>32)&^uint64(0xfff))
	minor = uint32(st.Dev&0xff) | uint32((st.Dev>>12)&^uint64(0xff))
	return major, minor, nil
}

// writeCgroupFile writes content to a single cgroup control file.
func writeCgroupFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// ApplyCgroupBlkioLimits enforces blkio throttle limits for containerID on
// the device that backs xfsMountPoint. Zero values are skipped (no limit).
// The function probes both cgroupv1 and cgroupv2 paths automatically.
func ApplyCgroupBlkioLimits(containerID, xfsMountPoint string, readBPS, writeBPS, readIOPS, writeIOPS uint64) error {
	if readBPS == 0 && writeBPS == 0 && readIOPS == 0 && writeIOPS == 0 {
		return nil
	}
	maj, min, err := devMajorMinor(xfsMountPoint)
	if err != nil {
		return fmt.Errorf("cgroup blkio: %w", err)
	}
	cgDir, isV2, err := containerCgroupDir(containerID)
	if err != nil {
		return fmt.Errorf("cgroup blkio: %w", err)
	}
	dev := fmt.Sprintf("%d:%d", maj, min)
	if isV2 {
		// cgroupv2: single io.max entry covers all axes.
		line := dev
		if readBPS > 0 {
			line += fmt.Sprintf(" rbps=%d", readBPS)
		}
		if writeBPS > 0 {
			line += fmt.Sprintf(" wbps=%d", writeBPS)
		}
		if readIOPS > 0 {
			line += fmt.Sprintf(" riops=%d", readIOPS)
		}
		if writeIOPS > 0 {
			line += fmt.Sprintf(" wiops=%d", writeIOPS)
		}
		return writeCgroupFile(cgDir+"/io.max", line)
	}
	// cgroupv1: one file per axis.
	type axis struct {
		file string
		val  uint64
	}
	for _, a := range []axis{
		{"blkio.throttle.read_bps_device", readBPS},
		{"blkio.throttle.write_bps_device", writeBPS},
		{"blkio.throttle.read_iops_device", readIOPS},
		{"blkio.throttle.write_iops_device", writeIOPS},
	} {
		if a.val == 0 {
			continue
		}
		if err = writeCgroupFile(cgDir+"/"+a.file, fmt.Sprintf("%s %d", dev, a.val)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveCgroupBlkioLimits clears the blkio throttle limits previously set by
// ApplyCgroupBlkioLimits. Errors are only logged — this is best-effort on
// unmount; the container's cgroup may already be gone.
func RemoveCgroupBlkioLimits(containerID, xfsMountPoint string, readBPS, writeBPS, readIOPS, writeIOPS uint64) {
	if readBPS == 0 && writeBPS == 0 && readIOPS == 0 && writeIOPS == 0 {
		return
	}
	maj, min, err := devMajorMinor(xfsMountPoint)
	if err != nil {
		log.Warn("cgroup blkio remove: ", err)
		return
	}
	cgDir, isV2, err := containerCgroupDir(containerID)
	if err != nil {
		log.Warn("cgroup blkio remove: ", err)
		return
	}
	dev := fmt.Sprintf("%d:%d", maj, min)
	if isV2 {
		if err = writeCgroupFile(cgDir+"/io.max", dev+" rbps=max wbps=max riops=max wiops=max"); err != nil {
			log.Warn("cgroup blkio remove v2: ", err)
		}
		return
	}
	// cgroupv1: write 0 to clear each throttle.
	for _, file := range []string{
		"blkio.throttle.read_bps_device",
		"blkio.throttle.write_bps_device",
		"blkio.throttle.read_iops_device",
		"blkio.throttle.write_iops_device",
	} {
		if err = writeCgroupFile(cgDir+"/"+file, dev+" 0"); err != nil {
			log.Warn("cgroup blkio remove v1 (", file, "): ", err)
		}
	}
}
