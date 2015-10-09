package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-catalog-service/manager"
	"github.com/rancher/rancher-catalog-service/service"
	"net/http"
)

func main() {
	log.Infof("Starting Rancher Catalog service")

	manager.SetEnv()
	manager.Init()

	router := service.NewRouter()
	log.Fatal(http.ListenAndServe(":8088", router))
}
