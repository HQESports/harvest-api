package pubg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"harvest/internal/cache"
	"harvest/internal/config"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Client represents a PUBG API client
type Client struct {
	httpClient    *http.Client
	apiKey        string
	baseURL       string
	OldEnoughMin  int
	requestTicker *time.Ticker
	requestChan   chan struct{}
	cache         cache.Cache
	defaultTTL    time.Duration
}

const (
	SteamPlatform = "steam"
	EventPlatform = "tournaments"
)

// New creates a new PUBG API client with rate limiting and caching
func New(config config.PUBGConfig, cache cache.Cache) *Client {
	// Calculate interval between requests
	interval := time.Minute / time.Duration(config.RequestsPerMinute-1)

	log.Info().
		Int("requests_per_minute", config.RequestsPerMinute).
		Dur("request_interval", interval).
		Str("base_url", config.BaseURL).
		Bool("cache_enabled", cache != nil).
		Msg("Initializing PUBG API client")

	// Create a ticker that ticks once per allowed request
	ticker := time.NewTicker(interval)

	// Create a buffered channel to handle request permissions
	requestChan := make(chan struct{}, 1) // Buffer of 1 allows one immediate request

	// Initialize the channel with one token
	requestChan <- struct{}{}

	// Start goroutine to add tokens at the specified rate
	go func() {
		for range ticker.C {
			// Try to add a token, but don't block if buffer is full
			select {
			case requestChan <- struct{}{}:
				// Token added
			default:
				// Buffer full, skip
			}
		}
	}()

	// If no default TTL is specified, set a reasonable default
	defaultTTL := time.Duration(config.DefaultCacheTTL) * time.Minute
	if defaultTTL == 0 {
		defaultTTL = time.Duration(15) * time.Minute
	}

	client := &Client{
		httpClient:    &http.Client{Timeout: time.Second * 30},
		apiKey:        config.APIKey,
		baseURL:       config.BaseURL,
		OldEnoughMin:  config.OldEnoughMin,
		requestTicker: ticker,
		requestChan:   requestChan,
		cache:         cache,
		defaultTTL:    defaultTTL,
	}

	return client
}

// generateCacheKey creates a unique hash for the request
func generateCacheKey(endpoint string) string {
	hash := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(hash[:])
}

// request is the internal method that makes an HTTP request
// with optional rate limiting and caching
func (c *Client) request(ctx context.Context, endpoint string, shouldRateLimit bool) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	startTime := time.Now()

	// Try to get from cache first if cache is enabled
	if c.cache != nil {
		cacheKey := generateCacheKey(endpoint)
		cachedData, err := c.cache.Get(ctx, cacheKey)

		if err == nil {
			// Cache hit
			log.Debug().
				Str("endpoint", endpoint).
				Int("response_size", len(cachedData)).
				Msg("Cache hit")
			return cachedData, nil
		} else if err != cache.ErrCacheMiss {
			// Real error, not just a cache miss
			log.Warn().
				Err(err).
				Str("endpoint", endpoint).
				Msg("Cache error, falling back to API")
		}
	}

	// Apply rate limiting if needed
	if shouldRateLimit {
		<-c.requestChan
	}

	// Create request with headers
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", url).
			Msg("Error creating request")
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Add the context to the request
	req = req.WithContext(ctx)

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/vnd.api+json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", url).
			Msg("Error executing request")
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", url).
			Int("status_code", resp.StatusCode).
			Msg("Error reading response body")
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseAPIError(resp.StatusCode, respBody)
		log.Error().
			Err(apiErr).
			Str("url", url).
			Int("status_code", resp.StatusCode).
			Msg("API error response")
		return nil, apiErr
	}

	// Cache the response if cache is enabled
	if c.cache != nil {
		cacheKey := generateCacheKey(endpoint)
		err := c.cache.Set(ctx, cacheKey, respBody, c.defaultTTL)
		if err != nil {
			log.Warn().
				Err(err).
				Str("endpoint", endpoint).
				Msg("Failed to cache response")
		}
	}

	// Log a single success entry with duration
	log.Debug().
		Str("endpoint", endpoint).
		Int("status_code", resp.StatusCode).
		Int("response_size", len(respBody)).
		Dur("duration", time.Since(startTime)).
		Msg("API request completed")

	return respBody, nil
}

// Request is a backward-compatible function that always applies rate limiting
func (c *Client) Request(endpoint string) ([]byte, error) {
	return c.RequestRateLimited(endpoint)
}

// RequestRateLimited makes a rate-limited request to the PUBG API
func (c *Client) RequestRateLimited(endpoint string) ([]byte, error) {
	return c.request(context.Background(), endpoint, true)
}

// RequestRateLimitedWithContext makes a rate-limited request with context
func (c *Client) RequestRateLimitedWithContext(ctx context.Context, endpoint string) ([]byte, error) {
	return c.request(ctx, endpoint, true)
}

// RequestNonRateLimited makes a request to the PUBG API without rate limiting
func (c *Client) RequestNonRateLimited(endpoint string) ([]byte, error) {
	return c.request(context.Background(), endpoint, false)
}

// RequestNonRateLimitedWithContext makes a non-rate-limited request with context
func (c *Client) RequestNonRateLimitedWithContext(ctx context.Context, endpoint string) ([]byte, error) {
	return c.request(ctx, endpoint, false)
}

// parseAPIError extracts error information from the API response
func parseAPIError(statusCode int, respBody []byte) error {
	var errResp struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &errResp); err == nil && len(errResp.Errors) > 0 {
		return fmt.Errorf("API error: %s - %s",
			errResp.Errors[0].Title,
			errResp.Errors[0].Detail)
	}

	return fmt.Errorf("API error: status code %d", statusCode)
}

// GetTelemetry retrieves a telemetry file with optional rate limiting, but no caching
// as telemetry files are too large to cache efficiently
func (c *Client) GetTelemetry(ctx context.Context, telemetryURL string, shouldRateLimit bool) ([]byte, error) {
	startTime := time.Now()

	// Apply rate limiting if needed
	if shouldRateLimit {
		<-c.requestChan
	}

	// Create the request
	req, err := http.NewRequest(http.MethodGet, telemetryURL, nil)
	if err != nil {
		log.Error().
			Err(err).
			Str("telemetry_url", telemetryURL).
			Msg("Error creating telemetry request")
		return nil, fmt.Errorf("error creating telemetry request: %w", err)
	}

	// Add the context to the request
	req = req.WithContext(ctx)

	// Set the authorization header with your bearer token
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("telemetry_url", telemetryURL).
			Msg("Error executing telemetry request")
		return nil, fmt.Errorf("error making telemetry request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Err(err).
			Str("telemetry_url", telemetryURL).
			Int("status_code", resp.StatusCode).
			Msg("Error reading telemetry response")
		return nil, fmt.Errorf("error reading telemetry response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseAPIError(resp.StatusCode, respBody)
		log.Error().
			Err(apiErr).
			Str("telemetry_url", telemetryURL).
			Int("status_code", resp.StatusCode).
			Msg("Telemetry API error response")
		return nil, apiErr
	}

	// Log a single success entry with duration
	log.Debug().
		Str("telemetry_url", telemetryURL).
		Int("status_code", resp.StatusCode).
		Int("response_size", len(respBody)).
		Dur("duration", time.Since(startTime)).
		Msg("Telemetry request completed")

	return respBody, nil
}

// Backward compatibility methods for telemetry

func (c *Client) GetTelemetryRateLimited(telemetryURL string) ([]byte, error) {
	return c.GetTelemetry(context.Background(), telemetryURL, true)
}

func (c *Client) GetTelemetryNonRateLimited(telemetryURL string) ([]byte, error) {
	return c.GetTelemetry(context.Background(), telemetryURL, false)
}

// GetTelemetryWithContext adds context support to telemetry requests
func (c *Client) GetTelemetryWithContext(ctx context.Context, telemetryURL string, shouldRateLimit bool) ([]byte, error) {
	return c.GetTelemetry(ctx, telemetryURL, shouldRateLimit)
}

// Close stops the ticker and closes the cache when the client is no longer needed
func (c *Client) Close() {
	log.Info().Msg("Shutting down PUBG API client")

	if c.requestTicker != nil {
		c.requestTicker.Stop()
	}

	if c.cache != nil {
		err := c.cache.Close()
		if err != nil {
			log.Error().Err(err).Msg("Error closing cache")
		}
	}
}
