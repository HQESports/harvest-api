package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// JobStatus represents the current state of a job
type JobStatus string

const (
	StatusQueued     JobStatus = "queued"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusRetrying   JobStatus = "retrying"
	StatusCancelled  JobStatus = "cancelled"
)

// JobResultType represents the outcome of a processed item
type JobResultType string

const (
	ResultSuccess JobResultType = "success"
	ResultWarning JobResultType = "warning"
	ResultFailure JobResultType = "failure"
)

// JobResult represents the outcome of processing a single item within a job
type JobResult struct {
	Type       JobResultType `bson:"type" json:"type"`
	Message    string        `bson:"message" json:"message"`
	Identifier string        `bson:"identifier,omitempty" json:"identifier,omitempty"`
	Timestamp  time.Time     `bson:"timestamp" json:"timestamp"`
}

// JobMetrics tracks the processing statistics for a job
type JobMetrics struct {
	TotalItems      int `bson:"total_items" json:"total_items"`
	ProcessedItems  int `bson:"processed_items" json:"processed_items"`
	SuccessCount    int `bson:"success_count" json:"success_count"`
	WarningCount    int `bson:"warning_count" json:"warning_count"`
	FailureCount    int `bson:"failure_count" json:"failure_count"`
	BatchesTotal    int `bson:"batches_total" json:"batches_total"`
	BatchesComplete int `bson:"batches_complete" json:"batches_complete"`
}

// Job represents a background processing task
type Job struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Type        string             `bson:"type" json:"type"`
	Status      JobStatus          `bson:"status" json:"status"`
	Progress    int                `bson:"progress" json:"progress"`
	Metrics     JobMetrics         `bson:"metrics" json:"metrics"`
	Payload     interface{}        `bson:"payload" json:"payload"`
	Results     []JobResult        `bson:"results" json:"results"`
	ErrorList   []string           `bson:"error_list,omitempty" json:"error_list,omitempty"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	CompletedAt *time.Time         `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	TokenID     string             `bson:"user_id" json:"user_id"`
	BatchSize   int                `bson:"batch_size" json:"batch_size"`
}
