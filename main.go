package main

import (
	"log"
	"strings"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/router"
	"github.com/basketikun/infinite-canvas/service"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatal(err)
	}
	if err := service.EnsureDefaultAdmin(); err != nil {
		log.Fatal(err)
	}
	service.SyncPDDLocalLibraries()
	service.StartPromptSyncScheduler()
	addr := config.Cfg.Port
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	log.Fatal(router.New().Run(addr))
}
