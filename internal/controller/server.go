package controller

import (
	"context"
	"harvest/internal/aws"
	"harvest/internal/cache"
	"harvest/internal/database"
	"harvest/internal/rabbitmq"
)

type ServerController interface {
	DBHealth() error
	CacheHealth() error
	RabbitHealth() error
	Online() string
	AWSFileServiceHealth() error
}

type serverController struct {
	db     database.Database
	cache  cache.Cache
	rabbit rabbitmq.Client
	fs     aws.FileService
}

func NewServer(db database.Database, cache cache.Cache, rabbit rabbitmq.Client, fileService aws.FileService) ServerController {
	return &serverController{
		db:     db,
		cache:  cache,
		rabbit: rabbit,
		fs:     fileService,
	}
}

func (sc *serverController) Online() string {
	return "Online"
}

func (sc *serverController) DBHealth() error {
	return sc.db.Health()
}

func (sc *serverController) CacheHealth() error {
	return sc.cache.Health(context.TODO())
}

func (sc *serverController) RabbitHealth() error {
	return sc.rabbit.Health()
}

func (sc *serverController) AWSFileServiceHealth() error {
	return sc.fs.TestConnection()
}
