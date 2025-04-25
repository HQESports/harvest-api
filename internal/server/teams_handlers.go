package server

import (
	"encoding/json"
	"harvest/internal/model"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TeamRequest represents the JSON request for creating/updating a team
type TeamRequest struct {
	Name        string         `json:"name" binding:"required"`
	EsportTag   string         `json:"esportTag" binding:"required"`
	SearchCount int            `json:"searchCount" binding:"required,min=2,max=4"`
	Players     []model.Player `json:"players" binding:"required"`
}

// TeamResponse represents the response for team operations
type TeamResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ImageURL    string         `json:"imageUrl"`
	EsportTag   string         `json:"esportTag"`
	SearchCount int            `json:"searchCount"`
	Players     []model.Player `json:"players"`
}

// CreateTeamHandler creates a new team
func (s *Server) CreateTeamHandler(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data: " + err.Error()})
		return
	}

	// Get team data from form
	var teamReq TeamRequest
	teamJSON := c.Request.FormValue("team")
	if err := json.Unmarshal([]byte(teamJSON), &teamReq); err != nil {
		log.Info().Msg(teamJSON)
		log.Error().Err(err).Msg("failed to parse JSON data from form value")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team data: " + err.Error()})
		return
	}

	// Create team model
	team := model.Team{
		ID:          primitive.NewObjectID(),
		Name:        teamReq.Name,
		EsportTag:   teamReq.EsportTag,
		SearchCount: teamReq.SearchCount,
		Players:     teamReq.Players,
	}

	// Get image file if provided
	var imageFile io.Reader
	var imageFileName string
	file, header, err := c.Request.FormFile("image")
	if err == nil {
		defer file.Close()
		imageFile = file
		imageFileName = header.Filename
	}

	// Create the team
	if err := s.teamController.CreateTeam(c.Request.Context(), &team, imageFile, imageFileName); err != nil {
		log.Error().Err(err).Str("teamName", team.Name).Msg("Failed to create team")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create team: " + err.Error()})
		return
	}

	// Convert to response format
	response := convertTeamToResponse(&team)
	c.JSON(http.StatusCreated, response)
}

// GetTeamHandler returns a specific team by ID
func (s *Server) GetTeamHandler(c *gin.Context) {
	idStr := c.Param("id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID format"})
		return
	}

	// Get the team
	team, err := s.teamController.GetTeamByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get team: " + err.Error()})
		return
	}

	if team == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Team not found"})
		return
	}

	// Convert to response format
	response := convertTeamToResponse(team)
	c.JSON(http.StatusOK, response)
}

// UpdateTeamHandler updates an existing team
func (s *Server) UpdateTeamHandler(c *gin.Context) {
	idStr := c.Param("id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID format"})
		return
	}

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data: " + err.Error()})
		return
	}

	// Get team data from form
	var teamReq TeamRequest
	teamJSON := c.Request.FormValue("team")
	if err := json.Unmarshal([]byte(teamJSON), &teamReq); err != nil {
		log.Error().Err(err).Msg("failed to parse JSON data from form value")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team data: " + err.Error()})
		return
	}

	// Create team model with ID from path
	team := model.Team{
		ID:          id,
		Name:        teamReq.Name,
		EsportTag:   teamReq.EsportTag,
		SearchCount: teamReq.SearchCount,
		Players:     teamReq.Players,
	}

	// Get image file if provided
	var imageFile io.Reader
	var imageFileName string
	file, header, err := c.Request.FormFile("image")
	if err == nil {
		defer file.Close()
		imageFile = file
		imageFileName = header.Filename
	}

	// Update the team
	if err := s.teamController.UpdateTeam(c.Request.Context(), &team, imageFile, imageFileName); err != nil {
		log.Error().Err(err).Str("teamID", idStr).Msg("Failed to update team")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update team: " + err.Error()})
		return
	}

	// Convert to response format
	response := convertTeamToResponse(&team)
	c.JSON(http.StatusOK, response)
}

// ListTeamsHandler returns a list of all teams
func (s *Server) ListTeamsHandler(c *gin.Context) {
	// List all teams
	teams, err := s.teamController.ListTeams(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list teams: " + err.Error()})
		return
	}

	// Convert to response format
	responses := make([]TeamResponse, len(teams))
	for i, team := range teams {
		responses[i] = convertTeamToResponse(&team)
	}

	c.JSON(http.StatusOK, responses)
}

// DeleteTeamHandler deletes a team by ID
func (s *Server) DeleteTeamHandler(c *gin.Context) {
	idStr := c.Param("id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid team ID format"})
		return
	}

	// Delete the team
	if err := s.teamController.DeleteTeam(c.Request.Context(), id); err != nil {
		log.Error().Err(err).Str("teamID", idStr).Msg("Failed to delete team")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete team: " + err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// Helper function to convert a team model to a response format
func convertTeamToResponse(team *model.Team) TeamResponse {
	return TeamResponse{
		ID:          team.ID.Hex(),
		Name:        team.Name,
		ImageURL:    team.ImageURL,
		EsportTag:   team.EsportTag,
		SearchCount: team.SearchCount,
		Players:     team.Players,
	}
}
