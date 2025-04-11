package server

import (
	"fmt"
	"harvest/internal/config"
	"harvest/internal/controller"
	"harvest/internal/database"
	"harvest/pkg/pubg"
	"net/http"
	"time"
)

type Server struct {
	sc     controller.ServerController
	pc     controller.PubgController
	config config.Config
}

func New(config config.Config, db database.Database, client pubg.Client) *http.Server {
	sc := controller.NewServer(db)
	pc := controller.NewPUBG(db, client)

	server := Server{
		sc:     sc,
		pc:     pc,
		config: config,
	}

	return &http.Server{
		Addr:         fmt.Sprintf(":%v", config.Port),
		Handler:      server.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}
