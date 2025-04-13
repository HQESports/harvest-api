// internal/controller/jobs_controller.go

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"harvest/internal/config"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/processor"
	"harvest/internal/rabbitmq"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// JobController handles job operations
type JobController interface {
	// CreateJob creates a new job and enqueues it for processing
	CreateJob(ctx context.Context, jobType string, payload interface{}, userID string) (*model.Job, error)

	// GetJob retrieves a job by ID
	GetJob(ctx context.Context, jobID string) (*model.Job, error)

	// ListJobs lists jobs with pagination
	ListJobs(ctx context.Context, userID string, limit, offset int) ([]*model.Job, error)

	// ListJobsByStatus lists jobs by status with pagination
	ListJobsByStatus(ctx context.Context, status model.JobStatus, limit, offset int) ([]*model.Job, error)

	// ListJobsByStatusAndUser lists jobs by status and user with pagination
	ListJobsByStatusAndUser(ctx context.Context, status model.JobStatus, userID string, limit, offset int) ([]*model.Job, error)

	// UpdateJobProgress updates a job's progress and metrics
	UpdateJobProgress(ctx context.Context, jobID string, progress int, metrics model.JobMetrics) error

	// AddJobResults adds results to a job
	AddJobResults(ctx context.Context, jobID string, results []model.JobResult) error

	// AddJobError adds an error to a job
	AddJobError(ctx context.Context, jobID string, errorMsg string) error

	// ProcessJobs starts consuming and processing jobs
	ProcessJobs(ctx context.Context) error

	// Get Available Job Types
	GetAvailableJobTypes() map[string]string

	// StopProcessing stops the job processing
	StopProcessing()
}

// jobController implements JobController
type jobController struct {
	db              database.JobDatabase
	rabbitClient    rabbitmq.Client
	rabbitConfig    config.RabbitMQConfig
	jobsConfig      config.JobsConfig
	processRegistry processor.BatchRegistry
	consumerTag     string
	shutdown        chan struct{}
	wg              sync.WaitGroup
}

// NewJobController creates a new job controller
func NewJobController(db database.JobDatabase, rabbitClient rabbitmq.Client,
	rabbitConfig config.RabbitMQConfig, jobsConfig config.JobsConfig, registry processor.BatchRegistry) JobController {
	return &jobController{
		db:              db,
		rabbitClient:    rabbitClient,
		rabbitConfig:    rabbitConfig,
		jobsConfig:      jobsConfig,
		processRegistry: registry,
		shutdown:        make(chan struct{}),
	}
}

// CreateJob creates a new job and enqueues it
func (c *jobController) CreateJob(ctx context.Context, jobType string, payload interface{}, tokenID string) (*model.Job, error) {
	// Create a new job
	job := &model.Job{
		ID:        primitive.NewObjectID(),
		Type:      jobType,
		Status:    model.StatusQueued,
		Progress:  0,
		Payload:   payload, // Store the payload as-is for workers to use
		Results:   []model.JobResult{},
		ErrorList: []string{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		TokenID:   tokenID,
		BatchSize: getBatchSize(c.jobsConfig, jobType),
		Metrics: model.JobMetrics{
			TotalItems:      0,
			ProcessedItems:  0,
			SuccessCount:    0,
			WarningCount:    0,
			FailureCount:    0,
			BatchesTotal:    0,
			BatchesComplete: 0,
		},
	}

	// Save the job to the database
	err := c.db.CreateJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	// Enqueue the job
	err = c.enqueueJob(job)
	if err != nil {
		// Update job status to failed if enqueueing fails
		c.db.UpdateJobStatus(ctx, job.ID.Hex(), model.StatusFailed, fmt.Sprintf("Failed to enqueue job: %s", err.Error()))
		return job, fmt.Errorf("failed to enqueue job: %w", err)
	}

	log.Info().
		Str("jobId", job.ID.Hex()).
		Str("jobType", jobType).
		Msg("Job created and enqueued")

	return job, nil
}

// enqueueJob publishes a job to RabbitMQ
func (c *jobController) enqueueJob(job *model.Job) error {
	// Single queue for all jobs
	queueName := c.rabbitConfig.QueueName

	// Create message headers
	headers := amqp.Table{
		"job_id":   job.ID.Hex(),
		"job_type": job.Type,
	}

	// Create a simplified message payload (ID only - the full job is in MongoDB)
	message := map[string]string{
		"job_id": job.ID.Hex(),
	}

	// Convert message to JSON
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Publish the message to RabbitMQ
	err = c.rabbitClient.Publish(
		c.rabbitConfig.ExchangeName,
		queueName, // Using queue name as routing key
		messageBytes,
		headers,
	)

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// GetJob retrieves a job by ID
func (c *jobController) GetJob(ctx context.Context, jobID string) (*model.Job, error) {
	return c.db.GetJobByID(ctx, jobID)
}

// ListJobs lists jobs with pagination
func (c *jobController) ListJobs(ctx context.Context, userID string, limit, offset int) ([]*model.Job, error) {
	return c.db.ListJobsByUserID(ctx, userID, limit, offset)
}

// ListJobsByStatus lists jobs by status with pagination
func (c *jobController) ListJobsByStatus(ctx context.Context, status model.JobStatus, limit, offset int) ([]*model.Job, error) {
	return c.db.ListJobsByStatus(ctx, status, limit, offset)
}

// ListJobsByStatusAndUser lists jobs by status and user with pagination
func (c *jobController) ListJobsByStatusAndUser(ctx context.Context, status model.JobStatus, userID string, limit, offset int) ([]*model.Job, error) {
	return c.db.ListJobsByStatusAndUserID(ctx, status, userID, limit, offset)
}

// UpdateJobProgress updates a job's progress and metrics
func (c *jobController) UpdateJobProgress(ctx context.Context, jobID string, progress int, metrics model.JobMetrics) error {
	return c.db.UpdateJobProgress(ctx, jobID, progress, metrics)
}

// AddJobResults adds results to a job
func (c *jobController) AddJobResults(ctx context.Context, jobID string, results []model.JobResult) error {
	return c.db.AddJobResults(ctx, jobID, results)
}

// AddJobError adds an error to a job
func (c *jobController) AddJobError(ctx context.Context, jobID string, errorMsg string) error {
	return c.db.AddJobError(ctx, jobID, errorMsg)
}

// ProcessJobs starts consuming jobs from RabbitMQ
func (c *jobController) ProcessJobs(ctx context.Context) error {
	// Check if we have any registered processors
	if len(c.processRegistry.AvailableProcessors()) == 0 {
		return fmt.Errorf("no job processors registered")
	}

	// Single queue for all jobs
	queueName := "jobs"

	// Ensure the exchange exists
	err := c.rabbitClient.DeclareExchange(c.rabbitConfig.ExchangeName, "direct")
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Ensure the queue exists
	queue, err := c.rabbitClient.DeclareQueue(queueName)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", queueName, err)
	}

	// Bind the queue to the exchange
	err = c.rabbitClient.BindQueue(queueName, c.rabbitConfig.ExchangeName, queueName)
	if err != nil {
		return fmt.Errorf("failed to bind queue %s: %w", queueName, err)
	}

	// Start consumer for this queue
	c.consumerTag = fmt.Sprintf("jobs-consumer-%s", primitive.NewObjectID().Hex())
	c.startConsumer(ctx, queue.Name, c.consumerTag)

	log.Info().Int("processors", len(c.processRegistry.AvailableProcessors())).Msg("Job processing started")
	return nil
}

// StopProcessing stops all job consumers
func (c *jobController) StopProcessing() {
	close(c.shutdown)
	c.wg.Wait()
	log.Info().Msg("Job processing stopped")
}

// startConsumer starts a consumer for the jobs queue
func (c *jobController) startConsumer(ctx context.Context, queueName, consumerTag string) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		log.Info().
			Str("queue", queueName).
			Str("consumerTag", consumerTag).
			Msg("Starting job consumer")

		for {
			select {
			case <-ctx.Done():
				log.Info().
					Str("consumerTag", consumerTag).
					Msg("Context cancelled, stopping consumer")
				return
			case <-c.shutdown:
				log.Info().
					Str("consumerTag", consumerTag).
					Msg("Shutdown signal received, stopping consumer")
				return
			default:
				// Continue processing
			}

			// Start consuming
			deliveries, err := c.rabbitClient.Consume(queueName, consumerTag)
			if err != nil {
				log.Error().
					Err(err).
					Str("queue", queueName).
					Str("consumerTag", consumerTag).
					Msg("Failed to consume from queue")

				// Wait before retrying
				time.Sleep(5 * time.Second)
				continue
			}

			// Process deliveries
			for delivery := range deliveries {
				c.processDelivery(ctx, delivery)
			}

			// If we reach here, the channel was closed
			log.Warn().
				Str("queue", queueName).
				Str("consumerTag", consumerTag).
				Msg("Consumer channel closed, reconnecting...")

			// Wait before reconnecting
			time.Sleep(5 * time.Second)
		}
	}()
}

// processDelivery handles a single delivery
func (c *jobController) processDelivery(ctx context.Context, delivery amqp.Delivery) {
	// Extract job ID from message headers
	jobID, ok := delivery.Headers["job_id"].(string)
	if !ok {
		log.Error().Msg("Message missing job_id header, rejecting")
		delivery.Nack(false, false) // Don't requeue malformed messages
		return
	}

	// Extract job type from message headers
	jobType, ok := delivery.Headers["job_type"].(string)
	if !ok {
		log.Error().Str("jobId", jobID).Msg("Message missing job_type header, rejecting")
		delivery.Nack(false, false) // Don't requeue malformed messages
		return
	}

	logger := log.With().
		Str("jobId", jobID).
		Str("jobType", jobType).
		Logger()

	logger.Info().Msg("Processing job message")

	// Get the job from the database
	job, err := c.db.GetJobByID(ctx, jobID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to retrieve job from database")
		delivery.Nack(false, false)
		return
	}

	// Get the processor for this job type
	processor, exists := c.processRegistry.Get(jobType)
	if !exists {
		logger.Error().Msg("No processor registered for job type")
		c.db.UpdateJobStatus(ctx, jobID, model.StatusFailed, "No processor registered for job type")
		delivery.Ack(false)
		return
	}

	// Update job status to processing
	err = c.db.UpdateJobStatus(ctx, jobID, model.StatusProcessing, "")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update job status to processing")
		delivery.Nack(false, false)
		return
	}

	// Process the job
	_, processingErr := processor.ProcessBatch(ctx, job)

	// Update job based on processing result
	if processingErr != nil {
		logger.Error().Err(processingErr).Msg("Job processing failed")
		failErr := c.db.UpdateJobStatus(ctx, jobID, model.StatusFailed, processingErr.Error())
		if failErr != nil {
			logger.Error().Err(failErr).Msg("Failed to update job status to failed")
		}
	} else {
		// Job processed successfully
		completeErr := c.db.UpdateJobStatus(ctx, jobID, model.StatusCompleted, "")
		if completeErr != nil {
			logger.Error().Err(completeErr).Msg("Failed to update job status to completed")
		}
		logger.Info().Msg("Job processed successfully")
	}

	// Acknowledge the message
	delivery.Ack(false)
}

// Helper function to get batch size for a job type
func getBatchSize(config config.JobsConfig, jobType string) int {
	for _, jt := range config.JobTypes {
		if jt.Type == jobType {
			return jt.BatchSize
		}
	}
	return config.DefaultBatchSize
}

func (c *jobController) GetAvailableJobTypes() map[string]string {
	jobTypeMap := make(map[string]string)

	for _, processType := range c.processRegistry.AvailableProcessors() {
		process, _ := c.processRegistry.Get(processType)
		jobTypeMap[processType] = process.Name()
	}

	return jobTypeMap
}
