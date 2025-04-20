package worker

import (
	"context"
	"harvest/internal/database"
	"harvest/internal/model"
	"harvest/internal/orchestrator"
	"harvest/pkg/pubg"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	PROCESS_MATCHES_TYPE        = "process_matches_worker"
	PROCESS_MATCHES_NAME        = "Process Matches Worker"
	PROCESS_MATCHES_DESCRIPTION = "Process each match stored and extract zone / plane path information"
)

type processMatchesWorker struct {
	pubgClient *pubg.Client
	db         database.Database

	ctx        *context.Context
	cancelFunc *context.CancelFunc
	jobID      *primitive.ObjectID
	cancelled  int32 // Using atomic for thread-safe access
}

// Cancel implements orchestrator.BatchWorker.
func (p *processMatchesWorker) Cancel() error {
	if p.cancelFunc != nil {
		cancelFunc := *p.cancelFunc
		cancelFunc()
	}

	// Set the cancelled flag atomically
	atomic.StoreInt32(&p.cancelled, 1)

	// Then clear the fields
	p.ctx = nil
	p.cancelFunc = nil
	p.jobID = nil

	return nil
}

// Description implements orchestrator.BatchWorker.
func (p *processMatchesWorker) Description() string {
	return PROCESS_MATCHES_DESCRIPTION
}

// IsActive implements job.BatchWorker.
func (p *processMatchesWorker) IsActive() bool {
	return atomic.LoadInt32(&p.cancelled) == 0 && p.ctx != nil
}

func (p *processMatchesWorker) ActiveJobID() *primitive.ObjectID {
	if !p.IsActive() {
		return nil
	}
	return p.jobID
}

// Name implements orchestrator.BatchWorker.
func (p *processMatchesWorker) Name() string {
	return PROCESS_MATCHES_NAME
}

// StartWorker implements orchestrator.BatchWorker.
func (p *processMatchesWorker) StartWorker(job *model.Job) (bool, error) {
	atomic.StoreInt32(&p.cancelled, 0)

	ctx, cancelFunc := context.WithCancel(context.Background())
	p.ctx = &ctx
	p.cancelFunc = &cancelFunc
	p.jobID = &job.ID

	defer func() {
		// Clean up in case of panic or return
		if !p.isCancelled() {
			p.ctx = nil
			p.cancelFunc = nil
			p.jobID = nil
		}
	}()

	// Take all tournament IDs and parse through them to build matches
	safeCtx := p.SafeContext()
	matches, err := p.db.GetUnProcessedMatches(safeCtx, 600)
	if err != nil {
		log.Error().Err(err).Msg("unable to get un-processed matches")
		return false, err
	}
	matchBatches := orchestrator.SplitIntoBatches(matches, 100)
	p.db.SetJobTotalBatches(p.SafeContext(), job.ID, len(matchBatches))

	for _, batch := range matchBatches {
		if p.isCancelled() {
			return true, nil
		}
		metrics := p.ProcessMatchBatch(batch)
		metrics.BatchesComplete += 1

		if p.isCancelled() {
			return true, nil
		}

		err := p.db.UpdateJobMetrics(p.SafeContext(), job.ID, metrics)

		if err != nil {
			return false, err
		}
	}

	return false, nil
}

func (p *processMatchesWorker) ProcessMatchBatch(batch []model.Match) model.JobMetrics {
	metrics := model.JobMetrics{
		ProcessedItems: len(batch),
	}

	if p.isCancelled() {
		return metrics
	}
	var wg sync.WaitGroup
	var mutex sync.Mutex

	updateMap := make(map[string]*model.TelemetryData, len(batch))

	for _, match := range batch {
		wg.Add(1)
		if p.isCancelled() {
			return metrics
		}

		go func(telemetryURL, matchID string) {
			defer wg.Done()

			if p.isCancelled() {
				return
			}

			if telemetryURL == "" {
				log.Warn().Str("matchID", matchID).Msg("empty telemetry URL, skipping match")
				mutex.Lock()
				metrics.WarningCount++
				mutex.Unlock()
				return
			}

			telemData, err := p.pubgClient.ProcessTelemetryFromURL(p.SafeContext(), telemetryURL)

			if err != nil {
				log.Error().Err(err).Str("Telemetry URL", telemetryURL).Msg("could not process telemetry URL")
				mutex.Lock()
				metrics.FailureCount++
				mutex.Unlock()
				return // Add this to exit early
			}

			// Check if we have valid data
			convertedData := ConvertTelemetryData(telemData)
			if convertedData == nil {
				log.Warn().Str("matchID", matchID).Msg("no valid telemetry data could be converted")
				mutex.Lock()
				metrics.WarningCount++
				mutex.Unlock()
				return // Add this to exit early
			}

			mutex.Lock()
			updateMap[matchID] = convertedData
			mutex.Unlock()
		}(match.TelemetryURL, match.MatchID)
	}
	wg.Wait()

	successCnt, err := p.db.UpdateMatchesWithTelemetryData(p.SafeContext(), updateMap)

	if err != nil {
		metrics.FailureCount++
	}

	metrics.SuccessCount += successCnt

	return metrics
}

// ConvertTelemetryData converts the PUBG API TelemetryData to the database model TelemetryData
func ConvertTelemetryData(apiData *pubg.TelemetryData) *model.TelemetryData {
	if apiData == nil {
		return nil
	}

	// Create database model telemetry data
	dbData := &model.TelemetryData{
		SafeZones: make([]model.SafeZone, 0, len(apiData.SafeZones)),
		PlanePath: model.PlanePath{
			StartX: apiData.PlanePath.StartPoint.X,
			StartY: apiData.PlanePath.StartPoint.Y,
			EndX:   apiData.PlanePath.EndPoint.X,
			EndY:   apiData.PlanePath.EndPoint.Y,
		},
	}

	// Convert safe zones
	for _, apiZone := range apiData.SafeZones {
		dbZone := model.SafeZone{
			Phase:  apiZone.Phase,
			X:      apiZone.X,
			Y:      apiZone.Y,
			Radius: apiZone.Radius,
		}
		dbData.SafeZones = append(dbData.SafeZones, dbZone)
	}

	return dbData
}

// Type implements orchestrator.BatchWorker.
func (p *processMatchesWorker) Type() string {
	return PROCESS_MATCHES_TYPE
}

// isCancelled returns true if the worker has been cancelled
func (p *processMatchesWorker) isCancelled() bool {
	return atomic.LoadInt32(&p.cancelled) == 1 || p.ctx == nil
}

func (p *processMatchesWorker) SafeContext() context.Context {
	if p.isCancelled() || p.ctx == nil {
		// Return a cancelled context if worker is cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return *p.ctx
}

func NewProcessMatchWorker(pubgClient *pubg.Client, db database.Database) orchestrator.BatchWorker {
	return &processMatchesWorker{
		pubgClient: pubgClient,
		db:         db,

		ctx:        nil,
		cancelFunc: nil,
		jobID:      nil,
		cancelled:  1,
	}
}
