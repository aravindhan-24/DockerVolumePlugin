package cmd

import (
	"context"
	"fmt"
	"os"

	"volume-plugin/handler"
	"volume-plugin/utils"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	ConfigFile            string
	VersionEnabled        bool
	VersionNumber         = 2.0
	DefaultConfigFilePath = "/etc/dockerplugins/VolumePlugin/config.json"
)

var RootCmd = &cobra.Command{
	Use:   "Docker Volume Plugin",
	Short: "Docker volume plugin for zoho",
	Long:  "XFS project quota-backed Docker volume plugin.",
	Run: func(cmd *cobra.Command, args []string) {
		if VersionEnabled {
			fmt.Printf("%.1f\n", VersionNumber)
			return
		}
		if _, err := os.Stat(ConfigFile); os.IsNotExist(err) {
			fmt.Printf("config file not found: %s\n", ConfigFile)
			os.Exit(1)
		}
		Config := utils.InitVar(ConfigFile)
		appHandler := handler.NewHandler(context.Background(), Config)
		log.Info("Docker volume plugin up and running")
		if Config.IsUnixSocket {
			socketAddress := Config.SocketAddress + Config.DriverName + ".sock"
			log.Info("starting UNIX socket at: ", socketAddress)
			log.Info(appHandler.ServeUnix(socketAddress, 0))
		} else {
			log.Info("starting TCP server on port: ", Config.PluginPort)
			log.Info(appHandler.ServeTCP(Config.DriverName, ":"+Config.PluginPort, "/run/docker/plugins/", nil))
		}
	},
}
