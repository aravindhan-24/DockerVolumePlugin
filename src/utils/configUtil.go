package utils

import (
	"fmt"
	"strconv"
)

type ConfigVariables struct {
	DriverName            string `json:"driver"`
	IsUnixSocket          bool   `json:"isUnixSocket"`
	SocketAddress         string `json:"socketAddress"`
	PluginPort            string `json:"pluginPort"`
	DefaultXFSMountPoint  string `json:"defaultXFSMountPoint"`
	DefaultBlockHardLimit string `json:"defaultBlockHardLimit"`
	DefaultBlockSoftLimit string `json:"defaultBlockSoftLimit"`
	DefaultInodeHardLimit string `json:"defaultInodeHardLimit"`
	DefaultInodeSoftLimit string `json:"defaultInodeSoftLimit"`
	// Cgroup blkio defaults — "0" means no limit.
	DefaultReadBPS   string `json:"defaultReadBPS"`
	DefaultWriteBPS  string `json:"defaultWriteBPS"`
	DefaultReadIOPS  string `json:"defaultReadIOPS"`
	DefaultWriteIOPS string `json:"defaultWriteIOPS"`
	MountPointDirMode     string `json:"mntptDirMode"`
	StatePath             string `json:"statepath"`
	OwnerOfMountPoint     string `json:"ownerOfMountPoint"`
}

// ParseOptionsAndConfig parses options and config to create a resultant map.
func ParseOptionsAndConfig(options map[string]string, config *ConfigVariables) (map[string]uint64, error) {
	resultMap := make(map[string]uint64)
	var err error
	parseAndAdd := func(key string, defaultValue interface{}) (err error) {
		value, exists := options[key]
		if exists {
			value, err := getByteSize(value)
			if err != nil {
				fmt.Println("Unable to convert the given number into byte")
				return err
			}
			resultMap[key] = value
		} else {
			resultMap[key] = defaultValue.(uint64)
		}
		return nil
	}
	defaultBHL, err := getByteSize(config.DefaultBlockHardLimit)
	if err != nil {
		return resultMap, err
	}
	defaultBSL, err := getByteSize(config.DefaultBlockSoftLimit)
	if err != nil {
		return resultMap, err
	}
	err = parseAndAdd("BlockHardLimit", defaultBHL)
	if err != nil {
		return resultMap, err
	}
	err = parseAndAdd("BlockSoftLimit", defaultBSL)
	if err != nil {
		return resultMap, err
	}
	DIHL, err := strconv.ParseUint(config.DefaultInodeHardLimit, 10, 64)
	if err != nil {
		return resultMap, err
	}
	DISL, err := strconv.ParseUint(config.DefaultInodeSoftLimit, 10, 64)
	if err != nil {
		return resultMap, err
	}
	err = parseAndAdd("BlockInodeHardLimit", DIHL)
	if err != nil {
		return resultMap, err
	}
	err = parseAndAdd("BlockInodeSoftLimit", DISL)
	if err != nil {
		return resultMap, err
	}
	return resultMap, nil
}
