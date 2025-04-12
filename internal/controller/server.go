package controller

import (
	"harvest/internal/cache"
	"harvest/internal/database"
)

type ServerController interface {
	Health() (string, error)
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

func (sc *serverController) Health() (string, error) {
	return sc.db.Health()
}
