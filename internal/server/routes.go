package server

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     s.config.CORS.AllowedOrigins,
		AllowMethods:     s.config.CORS.AllowedMethods,
		AllowHeaders:     s.config.CORS.AllowedHeaders,
		AllowCredentials: s.config.CORS.AllowCredentials,
		MaxAge:           time.Duration(s.config.CORS.MaxAge) * time.Second,
	}))

	r.GET("/health", s.healthHandler)
	r.GET("/online", s.onlineHandler)

	r.POST("/names", s.namesHandler)
	r.POST("/buildMatches", s.BuildMatchesFromFilter)

	r.POST("/tournaments", s.tournamentsHandler)

	r.POST("/expandPlayers", s.expandPlayers)

	return r
}
