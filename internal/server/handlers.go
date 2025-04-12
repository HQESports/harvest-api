package server

import (
	"fmt"
	"harvest/internal/controller"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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

func (s *Server) BuildMatchesFromFilter(c *gin.Context) {
	// Create a filter struct to bind the request body
	var filter controller.MatchFilter

	// Bind JSON from request body to the filter struct
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate map name if provided
	if filter.MapName != "" {
		if !ValidMapNames[filter.MapName] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid map name",
			})
			return
		}
	}

	// Validate match type if provided
	if filter.MatchType != "" {
		filter.MatchType = strings.ToLower(filter.MatchType)
		if !ValidMatchTypes[filter.MatchType] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid match type. Must be 'live' or 'event'",
			})
			return
		}
	}

	// TODO: Add logic to search through players and build out match IDs in the match collection

	// Return the filtered matches as JSON
	c.JSON(http.StatusCreated, gin.H{"numMatches": 0, "successful": 0})
}

func (s *Server) tournamentsHandler(c *gin.Context) {
	// Call the CreateTournaments function from the PUBG controller
	count, err := s.pc.CreateTournaments()

	if err != nil {
		log.Error().Msgf("Error creating tournament entities: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success response with count of processed tournaments
	c.JSON(http.StatusCreated, gin.H{"tournamentsProcessed": count})
}

func (s *Server) expandPlayers(c *gin.Context) {
	// Get the limit parameter from the route
	limitStr := c.Query("limit")
	limit, err := strconv.Atoi(limitStr)

	if err != nil {
		log.Error().Msgf("Invalid limit parameter: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	// Pass the limit to SearchAndExpandPlayers
	count, err := s.pc.SearchAndExpandPlayers(limit)

	if err != nil {
		log.Error().Msgf("Error searching and expanding players: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"playersUpserted": count})
}
