package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) namesHandler(c *gin.Context) {
	// Define a struct to match the request body format
	type NamesRequest struct {
		Names string `json:"names"`
	}

	var req NamesRequest

	// Bind the JSON request body to our struct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Split the comma-separated names string into an array
	names := strings.Split(req.Names, ",")

	// Trim any whitespace from each name
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}

	cnt, err := s.pc.CreatePlayers(names)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"playersProcessed": cnt})
}
