package database

import (
	"context"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// JobDatabase defines job-related database operations
type JobDatabase interface {
	// Create a new job
	CreateJob(ctx context.Context, job *model.Job) error

	// Get a job by ID
	GetJobByID(ctx context.Context, id primitive.ObjectID) (*model.Job, error)

	// Update a job's status
	UpdateJobStatus(ctx context.Context, id primitive.ObjectID, status model.JobStatus) error

	// List all jobs with optional filtering
	ListJobs(ctx context.Context) ([]model.Job, error)

	// Update a job's metrics and recalculate progress
	UpdateJobMetrics(ctx context.Context, id primitive.ObjectID, metrics model.JobMetrics) error

	// Set the total number of batches for a job
	SetJobTotalBatches(ctx context.Context, id primitive.ObjectID, totalBatches int) error

	// Increment the batches complete count
	IncrementJobBatchesComplete(ctx context.Context, id primitive.ObjectID, increment int) error
}

// CreateJob creates a new job in the database
func (m *mongoDB) CreateJob(ctx context.Context, job *model.Job) error {
	// Ensure the job has a valid ID
	if job.ID.IsZero() {
		job.ID = primitive.NewObjectID()
	}

	// Set creation and update times
	now := time.Now()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Insert the job
	_, err := m.jobsCol.InsertOne(ctx, job)
	if err != nil {
		log.Error().Err(err).Str("jobID", job.ID.Hex()).Msg("Failed to create job")
		return err
	}

	log.Debug().Str("jobID", job.ID.Hex()).Str("type", job.Type).Msg("Created new job")
	return nil
}

// GetJobByID retrieves a job from the database by its ID
func (m *mongoDB) GetJobByID(ctx context.Context, id primitive.ObjectID) (*model.Job, error) {
	var job model.Job

	filter := bson.M{"_id": id}
	err := m.jobsCol.FindOne(ctx, filter).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug().Str("jobID", id.Hex()).Msg("Job not found")
			return nil, nil
		}
		log.Error().Err(err).Str("jobID", id.Hex()).Msg("Failed to get job")
		return nil, err
	}

	log.Debug().Str("jobID", id.Hex()).Msg("Retrieved job")
	return &job, nil
}

// UpdateJobStatus updates the status of a job
func (m *mongoDB) UpdateJobStatus(ctx context.Context, id primitive.ObjectID, status model.JobStatus) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": now,
		},
	}

	// If the status is completed or failed, set the completed_at time
	if status == model.StatusCompleted || status == model.StatusFailed || status == model.StatusCancelled {
		update["$set"].(bson.M)["completed_at"] = now
	}

	result, err := m.jobsCol.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id.Hex()).Str("status", string(status)).Msg("Failed to update job status")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("jobID", id.Hex()).Msg("Job not found for status update")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("jobID", id.Hex()).Str("status", string(status)).Msg("Updated job status")
	return nil
}

// ListJobs retrieves all jobs from the database with optional filtering
func (m *mongoDB) ListJobs(ctx context.Context) ([]model.Job, error) {
	cursor, err := m.jobsCol.Find(ctx, bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list jobs")
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []model.Job
	if err = cursor.All(ctx, &jobs); err != nil {
		log.Error().Err(err).Msg("Failed to decode jobs")
		return nil, err
	}

	log.Debug().Int("count", len(jobs)).Msg("Retrieved jobs list")
	return jobs, nil
}

// UpdateJobMetrics increments the metrics of a job by the provided values
func (m *mongoDB) UpdateJobMetrics(ctx context.Context, id primitive.ObjectID, metrics model.JobMetrics) error {
	update := bson.M{
		"$inc": bson.M{
			"metrics.processed_items": metrics.ProcessedItems,
			"metrics.success_count":   metrics.SuccessCount,
			"metrics.failure_count":   metrics.FailureCount,
			"metrics.warning_count":   metrics.WarningCount,
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	result, err := m.jobsCol.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id.Hex()).Msg("Failed to increment job metrics")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("jobID", id.Hex()).Msg("Job not found for metrics update")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("jobID", id.Hex()).
		Int("processed_increment", metrics.ProcessedItems).
		Int("success_increment", metrics.SuccessCount).
		Int("failure_increment", metrics.FailureCount).
		Int("batches_increment", metrics.BatchesComplete).
		Msg("Incremented job metrics")
	return nil
}

// SetJobTotalBatches updates only the total_batches field of a job's metrics
func (m *mongoDB) SetJobTotalBatches(ctx context.Context, id primitive.ObjectID, totalBatches int) error {
	update := bson.M{
		"$set": bson.M{
			"metrics.total_batches": totalBatches,
			"updated_at":            time.Now(),
		},
	}

	result, err := m.jobsCol.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id.Hex()).Int("totalBatches", totalBatches).Msg("Failed to set job total batches")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("jobID", id.Hex()).Msg("Job not found for total batches update")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("jobID", id.Hex()).Int("totalBatches", totalBatches).Msg("Set job total batches")
	return nil
}

// IncrementJobBatchesComplete increments only the batches_complete field of a job's metrics
func (m *mongoDB) IncrementJobBatchesComplete(ctx context.Context, id primitive.ObjectID, increment int) error {
	update := bson.M{
		"$inc": bson.M{
			"metrics.batches_complete": increment,
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	result, err := m.jobsCol.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id.Hex()).Int("increment", increment).Msg("Failed to increment job batches complete")
		return err
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("jobID", id.Hex()).Msg("Job not found for batches complete increment")
		return mongo.ErrNoDocuments
	}

	log.Debug().Str("jobID", id.Hex()).Int("increment", increment).Msg("Incremented job batches complete")
	return nil
}
