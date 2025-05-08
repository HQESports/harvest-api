package server

import (
	"harvest/internal/controller"
	"harvest/internal/model"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	cnt, err := s.pc.CreatePlayers(c.Request.Context(), names)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"playersProcessed": cnt})
}

// filteredMatchesHandler handles requests for filtered PUBG matches
func (s *Server) filteredMatchesHandler(c *gin.Context) {
	// Parse map name from query parameters
	mapName := c.Query("map_name")
	if mapName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map_name query parameter is required"})
		return
	}

	// Parse match types (optional)
	var matchTypes []string
	matchTypesParam := c.Query("match_type")
	if matchTypesParam != "" {
		// Split comma-separated values
		types := strings.Split(matchTypesParam, ",")
		for _, t := range types {
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				matchTypes = append(matchTypes, trimmed)
			}
		}
	}

	// Parse start date (optional)
	var startDate *time.Time
	startDateParam := c.Query("start_date")
	if startDateParam != "" {
		parsed, err := time.Parse("2006-01-02", startDateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format. Use YYYY-MM-DD"})
			return
		}
		startDate = &parsed
	}

	// Parse end date (optional)
	var endDate *time.Time
	endDateParam := c.Query("end_date")
	if endDateParam != "" {
		parsed, err := time.Parse("2006-01-02", endDateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format. Use YYYY-MM-DD"})
			return
		}
		// Set to end of day (23:59:59)
		parsed = parsed.Add(24*time.Hour - time.Second)
		endDate = &parsed
	} else {
		// If no end date provided, use current time
		now := time.Now()
		endDate = &now
	}

	// Parse limit (optional)
	limit := 100000 // Default limit
	limitParam := c.Query("limit")
	if limitParam != "" {
		parsedLimit, err := strconv.Atoi(limitParam)
		if err != nil || parsedLimit < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter. Must be a positive integer"})
			return
		}
		limit = parsedLimit
	}

	// Create filter object
	filter := controller.MatchFilter{
		MapName:    mapName,
		MatchTypes: matchTypes,
		StartDate:  startDate,
		EndDate:    endDate,
		Limit:      limit,
	}

	// Get filtered matches
	matches, err := s.pc.GetFilteredMatches(c.Request.Context(), filter)
	if len(matches) == 0 {
		matches = []model.Match{}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return the matches
	c.JSON(http.StatusOK, gin.H{
		"matches": matches,
		"count":   len(matches),
		"filter":  filter,
	})
}

// filteredMatchesHandler handles requests for filtered PUBG matches
func (s *Server) filteredRandomMatchHandler(c *gin.Context) {
	// Parse map name from query parameters
	mapName := c.Query("map_name")
	if mapName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map_name query parameter is required"})
		return
	}

	// Parse match types (optional)
	var matchTypes []string
	matchTypesParam := c.Query("match_type")
	if matchTypesParam != "" {
		// Split comma-separated values
		types := strings.Split(matchTypesParam, ",")
		for _, t := range types {
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				matchTypes = append(matchTypes, trimmed)
			}
		}
	}

	// Parse start date (optional)
	var startDate *time.Time
	startDateParam := c.Query("start_date")
	if startDateParam != "" {
		parsed, err := time.Parse("2006-01-02", startDateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format. Use YYYY-MM-DD"})
			return
		}
		startDate = &parsed
	}

	// Parse end date (optional)
	var endDate *time.Time
	endDateParam := c.Query("end_date")
	if endDateParam != "" {
		parsed, err := time.Parse("2006-01-02", endDateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format. Use YYYY-MM-DD"})
			return
		}
		// Set to end of day (23:59:59)
		parsed = parsed.Add(24*time.Hour - time.Second)
		endDate = &parsed
	} else {
		// If no end date provided, use current time
		now := time.Now()
		endDate = &now
	}

	// Parse limit (optional)
	limit := 100000 // Default limit
	limitParam := c.Query("limit")
	if limitParam != "" {
		parsedLimit, err := strconv.Atoi(limitParam)
		if err != nil || parsedLimit < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter. Must be a positive integer"})
			return
		}
		limit = parsedLimit
	}

	// Create filter object
	filter := controller.MatchFilter{
		MapName:    mapName,
		MatchTypes: matchTypes,
		StartDate:  startDate,
		EndDate:    endDate,
		Limit:      limit,
	}

	// Get filtered matches
	match, err := s.pc.GetFilteredRandomMatch(c.Request.Context(), filter)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if match == nil {
		// Return the matches
		c.JSON(http.StatusOK, gin.H{
			"match":  model.Match{},
			"filter": filter,
		})
	}

	// Return the matches
	c.JSON(http.StatusOK, gin.H{
		"matches": match,
		"filter":  filter,
	})
}

func (s *Server) getPlayersHanlder(c *gin.Context) {
	pageParam := c.Query("page")
	page := 0
	if pageParam != "" {
		parsedPage, err := strconv.Atoi(pageParam)
		if err != nil || parsedPage < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter. Must be a non-negative integer"})
			return
		}
		page = parsedPage
	}
	// Parse page size from query parameters
	pageSizeParam := c.Query("page_size")
	pageSize := 25 // Default page size
	if pageSizeParam != "" {
		parsedPageSize, err := strconv.Atoi(pageSizeParam)
		if err != nil || parsedPageSize < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page_size parameter. Must be a positive integer"})
			return
		}
		pageSize = parsedPageSize
	}
	players, err := s.pc.GetPlayers(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(players) == 0 {
		players = []model.Entity{}
	}

	pageinationOptions := model.PaginationOptions{
		Page: page,
		Size: pageSize,
	}

	c.JSON(http.StatusOK, gin.H{
		"players":    players,
		"count":      len(players),
		"pagination": pageinationOptions,
	})
}

func (s *Server) getTournamentsHandler(c *gin.Context) {
	// Parse page number from query parameters
	pageParam := c.Query("page")
	page := 0
	if pageParam != "" {
		parsedPage, err := strconv.Atoi(pageParam)
		if err != nil || parsedPage < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter. Must be a non-negative integer"})
			return
		}
		page = parsedPage
	}
	// Parse page size from query parameters
	pageSizeParam := c.Query("page_size")
	pageSize := 25 // Default page size
	if pageSizeParam != "" {
		parsedPageSize, err := strconv.Atoi(pageSizeParam)
		if err != nil || parsedPageSize < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page_size parameter. Must be a positive integer"})
			return
		}
		pageSize = parsedPageSize
	}
	tournaments, err := s.pc.GetTournaments(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(tournaments) == 0 {
		tournaments = []model.Entity{}
	}

	pageinationOptions := model.PaginationOptions{
		Page: page,
		Size: pageSize,
	}

	c.JSON(http.StatusOK, gin.H{
		"tournaments": tournaments,
		"count":       len(tournaments),
		"pagination":  pageinationOptions,
	})
}

func (s *Server) GetMatchByIDHandler(c *gin.Context) {
	matchID := c.Param("match_id")
	if matchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match_id parameter is required"})
		return
	}

	match, err := s.pc.GetMatchByID(c.Request.Context(), matchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if match == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Match not found"})
		return
	}

	c.JSON(http.StatusOK, match)
}
