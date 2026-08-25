package handler

import (
	"context"

	"volume-plugin/store"
	"volume-plugin/utils"

	"github.com/docker/go-plugins-helpers/volume"
	log "github.com/sirupsen/logrus"
)

type storeAccess struct {
	store store.Datastore
}

func NewHandler(ctx1 context.Context, Config *utils.ConfigVariables) *volume.Handler {
	utils.InitDB(Config)
	// Clear stale block device references left from a previous run.
	utils.ClearBlockDev(Config)

	store, err := store.NewDB(Config.StatePath)
	if err != nil {
		log.Fatal(err)
	}
	Bean := &storeAccess{store}
	driver := IntializeXFSDriver(Bean, Config)
	driver.Context = ctx1
	log.Info("handler initialized")
	return volume.NewHandler(driver)
}
