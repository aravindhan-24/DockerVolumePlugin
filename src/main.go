package main

import "volume-plugin/cmd"

func main() {
	cmd.RootCmd.Flags().StringVarP(&cmd.ConfigFile, "config-file", "c", cmd.DefaultConfigFilePath, "Path to configuration file")
	cmd.RootCmd.Flags().BoolVarP(&cmd.VersionEnabled, "version", "v", false, "To show docker volume plugin version number")
	cmd.RootCmd.Execute()
}
