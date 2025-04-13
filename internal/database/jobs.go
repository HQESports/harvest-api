package database

import (
	"context"
	"errors"
	"harvest/internal/model"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// JobDatabase defines job-related database operations
type JobDatabase interface {
	// Create a new job
	CreateJob(ctx context.Context, job *model.Job) error

	// Get a job by ID
	GetJobByID(ctx context.Context, id string) (*model.Job, error)

	// Update a job
	UpdateJob(ctx context.Context, job *model.Job) error

	// Update job status
	UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errorMsg string) error

	// Update job progress
	UpdateJobProgress(ctx context.Context, id string, progress int, metrics model.JobMetrics) error

	// Add results to a job
	AddJobResults(ctx context.Context, id string, results []model.JobResult) error

	// Add error to a job
	AddJobError(ctx context.Context, id string, errorMsg string) error

	// List jobs by status
	ListJobsByStatus(ctx context.Context, status model.JobStatus, limit, offset int) ([]*model.Job, error)

	// List jobs by user ID
	ListJobsByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Job, error)

	// Count jobs by status
	CountJobsByStatus(ctx context.Context, status model.JobStatus) (int64, error)

	// List jobs by type
	ListJobsByType(ctx context.Context, jobType string, limit, offset int) ([]*model.Job, error)

	// List jobs by status and user ID
	ListJobsByStatusAndUserID(ctx context.Context, status model.JobStatus, userID string, limit, offset int) ([]*model.Job, error)
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

	// Initialize metrics if not already done
	if job.Metrics.TotalItems == 0 {
		job.Metrics.TotalItems = 0
		job.Metrics.ProcessedItems = 0
		job.Metrics.SuccessCount = 0
		job.Metrics.WarningCount = 0
		job.Metrics.FailureCount = 0
		job.Metrics.BatchesTotal = 0
		job.Metrics.BatchesComplete = 0
	}

	// Initialize empty slices if not already done
	if job.Results == nil {
		job.Results = []model.JobResult{}
	}

	if job.ErrorList == nil {
		job.ErrorList = []string{}
	}

	// Insert the job
	_, err := m.jobsCol.InsertOne(ctx, job)
	if err != nil {
		log.Error().Err(err).Str("jobID", job.ID.Hex()).Msg("Failed to create job")
		return err
	}

	log.Debug().Str("jobID", job.ID.Hex()).Str("type", job.Type).Msg("Created new job")
	return nil
}

// GetJobByID retrieves a job by its ID
func (m *mongoDB) GetJobByID(ctx context.Context, id string) (*model.Job, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var job model.Job
	err = m.jobsCol.FindOne(ctx, bson.M{"_id": objectID}).Decode(&job)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("job not found")
		}
		log.Error().Err(err).Str("jobID", id).Msg("Failed to get job")
		return nil, err
	}

	return &job, nil
}

// UpdateJob updates an entire job document
func (m *mongoDB) UpdateJob(ctx context.Context, job *model.Job) error {
	job.UpdatedAt = time.Now()

	_, err := m.jobsCol.ReplaceOne(
		ctx,
		bson.M{"_id": job.ID},
		job,
	)

	if err != nil {
		log.Error().Err(err).Str("jobID", job.ID.Hex()).Msg("Failed to update job")
		return err
	}

	log.Debug().Str("jobID", job.ID.Hex()).Str("status", string(job.Status)).Int("progress", job.Progress).Msg("Updated job")
	return nil
}

// UpdateJobStatus updates a job's status and optionally adds an error message
func (m *mongoDB) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errorMsg string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	// If there's an error message, add it to the error list
	if errorMsg != "" {
		update["$push"] = bson.M{
			"error_list": errorMsg,
		}
	}

	// If the job is completed, set the completed_at timestamp
	if status == model.StatusCompleted {
		now := time.Now()
		update["$set"].(bson.M)["completed_at"] = now
	}

	_, err = m.jobsCol.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id).Str("status", string(status)).Msg("Failed to update job status")
		return err
	}

	log.Debug().Str("jobID", id).Str("status", string(status)).Msg("Updated job status")
	return nil
}

// UpdateJobProgress updates a job's progress and metrics
func (m *mongoDB) UpdateJobProgress(ctx context.Context, id string, progress int, metrics model.JobMetrics) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"progress":   progress,
			"metrics":    metrics,
			"updated_at": time.Now(),
		},
	}

	_, err = m.jobsCol.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id).Int("progress", progress).Msg("Failed to update job progress")
		return err
	}

	log.Debug().Str("jobID", id).Int("progress", progress).Msg("Updated job progress")
	return nil
}

// AddJobResults adds results to a job's results array
func (m *mongoDB) AddJobResults(ctx context.Context, id string, results []model.JobResult) error {
	if len(results) == 0 {
		return nil
	}

	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	update := bson.M{
		"$push": bson.M{
			"results": bson.M{
				"$each": results,
			},
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	_, err = m.jobsCol.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id).Int("resultCount", len(results)).Msg("Failed to add job results")
		return err
	}

	log.Debug().Str("jobID", id).Int("resultCount", len(results)).Msg("Added job results")
	return nil
}

// AddJobError adds an error message to a job's error list
func (m *mongoDB) AddJobError(ctx context.Context, id string, errorMsg string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	update := bson.M{
		"$push": bson.M{
			"error_list": errorMsg,
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	}

	_, err = m.jobsCol.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id).Str("error", errorMsg).Msg("Failed to add job error")
		return err
	}

	log.Debug().Str("jobID", id).Str("error", errorMsg).Msg("Added job error")
	return nil
}

// ListJobsByStatus retrieves jobs by their status
func (m *mongoDB) ListJobsByStatus(ctx context.Context, status model.JobStatus, limit, offset int) ([]*model.Job, error) {
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(offset)).
		SetSort(bson.M{"created_at": -1})

	cursor, err := m.jobsCol.Find(ctx, bson.M{"status": status}, opts)
	if err != nil {
		log.Error().Err(err).Str("status", string(status)).Msg("Failed to list jobs by status")
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*model.Job
	if err := cursor.All(ctx, &jobs); err != nil {
		log.Error().Err(err).Msg("Failed to decode jobs")
		return nil, err
	}

	return jobs, nil
}

// ListJobsByUserID retrieves jobs by user ID
func (m *mongoDB) ListJobsByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Job, error) {
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(offset)).
		SetSort(bson.M{"created_at": -1})

	cursor, err := m.jobsCol.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Msg("Failed to list jobs by user ID")
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*model.Job
	if err := cursor.All(ctx, &jobs); err != nil {
		log.Error().Err(err).Msg("Failed to decode jobs")
		return nil, err
	}

	return jobs, nil
}

// CountJobsByStatus counts jobs with a specific status
func (m *mongoDB) CountJobsByStatus(ctx context.Context, status model.JobStatus) (int64, error) {
	count, err := m.jobsCol.CountDocuments(ctx, bson.M{"status": status})
	if err != nil {
		log.Error().Err(err).Str("status", string(status)).Msg("Failed to count jobs by status")
		return 0, err
	}

	return count, nil
}

// ListJobsByType retrieves jobs by their type
func (m *mongoDB) ListJobsByType(ctx context.Context, jobType string, limit, offset int) ([]*model.Job, error) {
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(offset)).
		SetSort(bson.M{"created_at": -1})

	cursor, err := m.jobsCol.Find(ctx, bson.M{"type": jobType}, opts)
	if err != nil {
		log.Error().Err(err).Str("type", jobType).Msg("Failed to list jobs by type")
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*model.Job
	if err := cursor.All(ctx, &jobs); err != nil {
		log.Error().Err(err).Msg("Failed to decode jobs")
		return nil, err
	}

	return jobs, nil
}

// ListJobsByStatusAndUserID retrieves jobs by both status and user ID
func (m *mongoDB) ListJobsByStatusAndUserID(ctx context.Context, status model.JobStatus, userID string, limit, offset int) ([]*model.Job, error) {
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(offset)).
		SetSort(bson.M{"created_at": -1})

	cursor, err := m.jobsCol.Find(ctx, bson.M{
		"status":  status,
		"user_id": userID,
	}, opts)
	if err != nil {
		log.Error().Err(err).Str("status", string(status)).Str("userID", userID).Msg("Failed to list jobs by status and user ID")
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*model.Job
	if err := cursor.All(ctx, &jobs); err != nil {
		log.Error().Err(err).Msg("Failed to decode jobs")
		return nil, err
	}

	return jobs, nil
}

// UpdateJobMetrics updates just the metrics for a job
func (m *mongoDB) UpdateJobMetrics(ctx context.Context, id string, metrics model.JobMetrics) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"metrics":    metrics,
			"updated_at": time.Now(),
		},
	}

	result, err := m.jobsCol.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Error().Err(err).Str("jobID", id).Msg("Failed to update job metrics")
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("job not found")
	}

	log.Debug().Str("jobID", id).
		Int("totalItems", metrics.TotalItems).
		Int("processedItems", metrics.ProcessedItems).
		Int("successCount", metrics.SuccessCount).
		Int("warningCount", metrics.WarningCount).
		Int("failureCount", metrics.FailureCount).
		Msg("Updated job metrics")
	return nil
}
