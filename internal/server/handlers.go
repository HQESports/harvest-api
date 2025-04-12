package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ValidMatchTypes defines allowed match types for efficient lookup
var ValidMatchTypes = map[string]bool{
	"live":  true,
	"event": true,
}

var ValidMapNames = map[string]bool{
	"Baltic_Main":     true,
	"Chimera_Main":    true,
	"Desert_Main":     true,
	"DihorOtok_Main":  true,
	"Erangel_Main":    true,
	"Heaven_Main":     true,
	"Kiki_Main":       true,
	"Range_Main":      true,
	"Savage_Main":     true,
	"Summerland_Main": true,
	"Tiger_Main":      true,
	"Neon_Main":       true,
}

// TODO: Implement context propigation to DB layer via controller route context -> controller -> DB. Add max times to HTTP server
func (s *Server) healthHandler(c *gin.Context) {
	health, err := s.sc.Health()

	if err != nil {
		c.String(http.StatusInternalServerError, health)
	}

	c.String(http.StatusOK, health)
}

func (s *Server) onlineHandler(c *gin.Context) {
	online := s.sc.Online()

	c.String(http.StatusOK, online)
}
