package server

import (
	"harvest/internal/model"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// JobRequest represents the request for creating a job
type JobRequest struct {
	Type    string      `json:"type" binding:"required"`
	Payload interface{} `json:"payload" binding:"required"`
}

// JobResponse represents the response for job operations
type JobResponse struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Progress  int               `json:"progress"`
	TokenID   string            `json:"tokenId"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
	Results   []model.JobResult `json:"results"`
	ErrorList []string          `json:"errorList,omitempty"`
	Metrics   model.JobMetrics  `json:"metrics"`
}

// CreateJobHandler creates a new job
func (s *Server) CreateJobHandler(c *gin.Context) {
	var req JobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get token ID from context (set by auth middleware)
	tokenID := getTokenID(c)
	if tokenID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token ID not found"})
		return
	}

	// Create the job
	job, err := s.jc.CreateJob(c.Request.Context(), req.Type, req.Payload, tokenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job: " + err.Error()})
		return
	}

	// Convert to response format
	response := convertJobToResponse(job)
	c.JSON(http.StatusCreated, response)
}

// GetJobHandler returns a specific job by ID
func (s *Server) GetJobHandler(c *gin.Context) {
	jobID := c.Param("id")

	// Get the job
	job, err := s.jc.GetJob(c.Request.Context(), jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get job: " + err.Error()})
		return
	}

	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	// Convert to response format
	response := convertJobToResponse(job)
	c.JSON(http.StatusOK, response)
}

// ListJobsHandler returns a list of jobs for the current token
func (s *Server) ListJobsHandler(c *gin.Context) {
	// Get pagination parameters
	limit, offset := getPaginationParams(c)

	// Get token ID from context
	tokenID := getTokenID(c)
	if tokenID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token ID not found"})
		return
	}

	// List jobs for this token
	jobs, err := s.jc.ListJobs(c.Request.Context(), tokenID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list jobs: " + err.Error()})
		return
	}

	// Convert to response format
	var response []JobResponse
	for _, job := range jobs {
		response = append(response, convertJobToResponse(job))
	}

	c.JSON(http.StatusOK, response)
}

// ListAllJobsHandler returns a list of all jobs in the system with pagination
func (s *Server) ListAllJobsHandler(c *gin.Context) {
	// Get pagination parameters
	limit, offset := getPaginationParams(c)

	// Get status from query parameter if provided
	statusParam := c.Query("status")
	if statusParam != "" {
		status := model.JobStatus(statusParam)
		// Validate status
		if !isValidJobStatus(status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job status"})
			return
		}

		// List jobs by status
		jobs, err := s.jc.ListJobsByStatus(c.Request.Context(), status, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list jobs by status: " + err.Error()})
			return
		}

		// Convert to response format
		var response []JobResponse
		for _, job := range jobs {
			response = append(response, convertJobToResponse(job))
		}

		c.JSON(http.StatusOK, response)
		return
	}

	// If no status filter, list all jobs (this would require a new controller method)
	// For now, we can just return an error suggesting to use a status filter
	c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide a status parameter"})
}

func (s *Server) ListAllAvailableJobTypes(c *gin.Context) {
	c.JSON(http.StatusOK, s.jc.GetAvailableJobTypes())
}

// Helper functions

// convertJobToResponse converts a job model to a response format
func convertJobToResponse(job *model.Job) JobResponse {
	return JobResponse{
		ID:        job.ID.Hex(),
		Type:      job.Type,
		Status:    string(job.Status),
		Progress:  job.Progress,
		TokenID:   job.TokenID, // Note: In your model, UserID is actually TokenID
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.Format(time.RFC3339),
		Metrics:   job.Metrics,
	}
}

// getPaginationParams extracts pagination parameters from request
func getPaginationParams(c *gin.Context) (int, int) {
	// Default values
	limit := 20
	offset := 0

	// Parse limit parameter if provided
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Parse offset parameter if provided
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	return limit, offset
}

// getTokenID gets the token ID from the context (set by auth middleware)
func getTokenID(c *gin.Context) string {
	tokenID, exists := c.Get("tokenID")
	if !exists {
		return ""
	}
	return tokenID.(string)
}

// isValidJobStatus checks if a job status is valid
func isValidJobStatus(status model.JobStatus) bool {
	validStatuses := []model.JobStatus{
		model.StatusQueued,
		model.StatusProcessing,
		model.StatusCompleted,
		model.StatusFailed,
	}

	for _, s := range validStatuses {
		if status == s {
			return true
		}
	}
	return false
}
