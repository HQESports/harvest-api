package server

import (
	"fmt"
	"harvest/internal/cache"
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
	tc     controller.TokenController
	config config.Config
}

func New(config config.Config, db database.Database, cache cache.Cache, client pubg.Client) *http.Server {
	sc := controller.NewServer(db, cache)
	pc := controller.NewPUBG(db, client)
	tc := controller.NewToken(db)

	server := Server{
		sc:     sc,
		pc:     pc,
		tc:     tc,
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
