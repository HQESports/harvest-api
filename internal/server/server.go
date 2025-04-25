package server

import (
	"context"
	"fmt"
	"harvest/internal/aws"
	"harvest/internal/cache"
	"harvest/internal/config"
	"harvest/internal/controller"
	"harvest/internal/database"
	"harvest/internal/orchestrator"
	"harvest/internal/rabbitmq"
	"harvest/pkg/pubg"
	"net/http"
	"time"
)

type Server struct {
	sc             controller.ServerController
	pc             controller.PubgController
	tc             controller.TokenController
	jc             controller.JobController
	mc             controller.MetricsController
	teamController controller.TeamController
	config         config.Config
}

func New(config config.Config, db database.Database, cache cache.Cache, rabbit rabbitmq.Client, client pubg.Client, workerRegistry orchestrator.WorkerRegistry, fileService aws.FileService) *http.Server {
	sc := controller.NewServer(db, cache, rabbit, fileService)

	jc := controller.NewJobController(db, rabbit, config.RabbitMQ, config.Jobs, workerRegistry)
	jc.ProcessJobs(context.Background()) // Starts consuming messages from rabbit MQ

	pc := controller.NewPUBG(db, client)

	mc := controller.NewMetricsController(db)

	tc := controller.NewToken(db)
	teamController := controller.NewTeamController(db, fileService)

	server := Server{
		sc:             sc,
		pc:             pc,
		tc:             tc,
		jc:             jc,
		mc:             mc,
		teamController: *teamController,
		config:         config,
	}

	return &http.Server{
		Addr:         fmt.Sprintf(":%v", config.Port),
		Handler:      server.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}
