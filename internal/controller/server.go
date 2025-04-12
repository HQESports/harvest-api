package controller

import (
	"context"
	"harvest/internal/cache"
	"harvest/internal/database"
)

type ServerController interface {
	DBHealth() (string, error)
	CacheHealth() (string, error)
	Online() string
}

type serverController struct {
	db    database.Database
	cache cache.Cache
}

func NewServer(db database.Database, cache cache.Cache) ServerController {
	return &serverController{
		db:    db,
		cache: cache,
	}
}

func (sc *serverController) Online() string {
	return "Online"
}

func (sc *serverController) DBHealth() (string, error) {
	return sc.db.Health()
}

func (sc *serverController) CacheHealth() (string, error) {
	return sc.cache.Ping(context.TODO())
}
