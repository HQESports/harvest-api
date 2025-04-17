package server

import (
	"harvest/internal/model"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// JobRequest represents the request for creating a job
type JobRequest struct {
	Type    string      `json:"type" binding:"required"`
	Payload interface{} `json:"payload" binding:"required"`
}

// JobResponse represents the response for job operations
type JobResponse struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	Status    string           `json:"status"`
	Progress  int              `json:"progress"`
	TokenID   string           `json:"tokenId"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
	ErrorList []string         `json:"errorList,omitempty"`
	Metrics   model.JobMetrics `json:"metrics"`
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

func (s *Server) CancelJobHandler(c *gin.Context) {
	jobType := c.Param("type")

	err := s.jc.CancelJob(jobType)
	if err != nil {
		log.Error().Err(err).Msg("Could not cancel a job")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully canceled job"})
}

// ListJobsHandler returns a list of jobs for the current token
func (s *Server) ListJobsHandler(c *gin.Context) {
	// List jobs for this token
	jobs, err := s.jc.ListJobs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list jobs: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
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
		TokenID:   job.TokenID, // Note: In your model, UserID is actually TokenID
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.Format(time.RFC3339),
		Metrics:   job.Metrics,
	}
}

// getTokenID gets the token ID from the context (set by auth middleware)
func getTokenID(c *gin.Context) string {
	tokenID, exists := c.Get("tokenID")
	if !exists {
		return ""
	}
	return tokenID.(string)
}
