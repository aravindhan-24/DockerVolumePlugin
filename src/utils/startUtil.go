package utils

import (
	"os"
	"path/filepath"

	"volume-plugin/store"
	xfsdriver "volume-plugin/xfs"

	"github.com/sirupsen/logrus"
)

func InitDB(Config *ConfigVariables) {
	store.InitStateFile(Config.StatePath)
}

func ClearBlockDev(Config *ConfigVariables) {
	blockDevicePath := filepath.Join(Config.DefaultXFSMountPoint, xfsdriver.BlockDeviceName)
	blockDeviceInfo, err := os.Lstat(blockDevicePath)
	if err != nil {
		logrus.Info(err)
		return
	}
	if blockDeviceInfo.Mode()&os.ModeDevice != 0 {
		os.Remove(blockDevicePath)
		logrus.Info("removed old block device")
	}
}
