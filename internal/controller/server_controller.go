package controller

import "harvest/internal/database"

type ServerController interface {
	Health() (string, error)
	Online() string
}

type serverController struct {
	db database.Database
}

func NewServer(db database.Database) ServerController {
	return &serverController{
		db: db,
	}
}

func (sc *serverController) Online() string {
	return "Online"
}

func (sc *serverController) Health() (string, error) {
	return sc.db.Health()
}
