package controller

import (
	"context"
	"harvest/internal/cache"
	"harvest/internal/database"
	"harvest/internal/rabbitmq"
)

type ServerController interface {
	DBHealth() error
	CacheHealth() error
	RabbitHealth() error
	Online() string
}

type serverController struct {
	db     database.Database
	cache  cache.Cache
	rabbit rabbitmq.Client
}

func NewServer(db database.Database, cache cache.Cache, rabbit rabbitmq.Client) ServerController {
	return &serverController{
		db:     db,
		cache:  cache,
		rabbit: rabbit,
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
