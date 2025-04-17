// internal/controller/jobs_controller.go

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"harvest/internal/config"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
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

	// ProcessJobs starts consuming and processing jobs
	ProcessJobs(ctx context.Context) error

	// Get Available Job Types
	GetAvailableJobTypes() map[string]string

	// Cancel Jobs by Type
	CancelJob(string) error

	// StopProcessing stops the job processing
	StopProcessing()

	// List all the jobs
	ListJobs(context.Context) ([]model.Job, error)

	GetJob(context.Context, string) (*model.Job, error)
}

// jobController implements JobController
type jobController struct {
	db              database.JobDatabase
	rabbitClient    rabbitmq.Client
	rabbitConfig    config.RabbitMQConfig
	jobsConfig      config.JobsConfig
	processRegistry orchestrator.WorkerRegistry
	consumerTag     string
	shutdown        chan struct{}
	wg              sync.WaitGroup
}

// NewJobController creates a new job controller
func NewJobController(db database.JobDatabase, rabbitClient rabbitmq.Client,
	rabbitConfig config.RabbitMQConfig, jobsConfig config.JobsConfig, registry orchestrator.WorkerRegistry) JobController {
	return &jobController{
		db:              db,
		rabbitClient:    rabbitClient,
		rabbitConfig:    rabbitConfig,
		jobsConfig:      jobsConfig,
		processRegistry: registry,
		shutdown:        make(chan struct{}),
	}
}

func (c *jobController) CancelJob(jobType string) error {
	worker, ok := c.processRegistry.Get(jobType)
	jobID := *worker.ActiveJobID()
	if !worker.IsActive() {
		return fmt.Errorf("job type is not active: %v", jobType)
	}

	if !ok {
		return fmt.Errorf("job type does not exist: %v", jobType)
	}

	err := worker.Cancel()
	if err != nil {
		return err
	}

	return c.db.UpdateJobStatus(context.TODO(), jobID, model.StatusCancelled)
}

func (c *jobController) GetJob(ctx context.Context, jobID string) (*model.Job, error) {
	jobIDPrim, err := primitive.ObjectIDFromHex(jobID)
	if err != nil {
		return nil, err
	}
	return c.db.GetJobByID(ctx, jobIDPrim)
}

// ListJob implements JobController.
func (c *jobController) ListJobs(ctx context.Context) ([]model.Job, error) {
	return c.db.ListJobs(ctx)
}

// CreateJob creates a new job and enqueues it
func (c *jobController) CreateJob(ctx context.Context, jobType string, payload interface{}, tokenID string) (*model.Job, error) {
	processor, ok := c.processRegistry.Get(jobType)
	if !ok {
		return nil, fmt.Errorf("job type not found in registry: %v", jobType)
	}

	if processor.IsActive() {
		return nil, fmt.Errorf("job type already active in registry cannot start")
	}

	// Create a new job
	job := &model.Job{
		ID:        primitive.NewObjectID(),
		Type:      jobType,
		Status:    model.StatusQueued,
		Payload:   payload, // Store the payload as-is for workers to use
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		TokenID:   tokenID,
		BatchSize: getBatchSize(c.jobsConfig, jobType),
		Metrics: model.JobMetrics{
			ProcessedItems:  0,
			SuccessCount:    0,
			WarningCount:    0,
			FailureCount:    0,
			BatchesComplete: 0,
			TotalBatches:    0,
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
		c.db.UpdateJobStatus(ctx, job.ID, model.StatusFailed)
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
	jobIDStr, ok := delivery.Headers["job_id"].(string)
	if !ok {
		log.Error().Msg("Message missing job_id header, rejecting")
		delivery.Nack(false, false) // Don't requeue malformed messages
		return
	}
	jobID, _ := primitive.ObjectIDFromHex(jobIDStr)

	// Extract job type from message headers
	jobType, ok := delivery.Headers["job_type"].(string)
	if !ok {
		log.Error().Str("jobId", jobID.Hex()).Msg("Message missing job_type header, rejecting")
		delivery.Nack(false, false) // Don't requeue malformed messages
		return
	}

	logger := log.With().
		Str("jobId", jobID.Hex()).
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
		c.db.UpdateJobStatus(ctx, jobID, model.StatusFailed)
		delivery.Ack(false)
		return
	}

	// Update job status to processing
	err = c.db.UpdateJobStatus(ctx, jobID, model.StatusProcessing)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update job status to processing")
		delivery.Nack(false, false)
		return
	}

	// Process the job
	cancelled, err := processor.StartWorker(job)

	// Update job based on processing result
	if err != nil {
		logger.Error().Err(err).Msg("Job processing failed")
		failErr := c.db.UpdateJobStatus(ctx, jobID, model.StatusFailed)
		if failErr != nil {
			logger.Error().Err(failErr).Msg("Failed to update job status to failed")
		}
	} else {
		// Job processed successfully
		status := model.StatusCompleted
		if cancelled {
			status = model.StatusCancelled
		}
		completeErr := c.db.UpdateJobStatus(ctx, jobID, status)
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
