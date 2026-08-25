package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
)

func InitVar(configFilePath string) *ConfigVariables {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("unable to get current user")
		os.Exit(1)
	}
	config := ConfigVariables{
		DriverName:            "VolumePlugin",
		IsUnixSocket:          true,
		SocketAddress:         "/run/docker/plugins/",
		PluginPort:            "8080",
		DefaultXFSMountPoint:  "",
		DefaultBlockHardLimit: "10k",
		DefaultBlockSoftLimit: "10k",
		DefaultInodeHardLimit: "500000",
		DefaultInodeSoftLimit: "500000",
		DefaultReadBPS:        "0",
		DefaultWriteBPS:       "0",
		DefaultReadIOPS:       "0",
		DefaultWriteIOPS:      "0",
		MountPointDirMode:     "0755",
		StatePath:             "/var/lib/VolumePlugin/state.json",
		OwnerOfMountPoint:     currentUser.Name,
	}
	bytestream, err := os.ReadFile(configFilePath)
	if err != nil {
		fmt.Println("error reading config file")
		os.Exit(1)
	}
	if err = json.Unmarshal(bytestream, &config); err != nil {
		fmt.Println("unable to parse config file")
		os.Exit(1)
	}
	return &config
}
