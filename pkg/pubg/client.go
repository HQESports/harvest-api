package pubg

import (
	"encoding/json"
	"fmt"
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
}

const (
	SteamPlatform = "steam"
	EventPlatform = "tournaments"
)

// New creates a new PUBG API client with rate limiting
func New(apiKey, baseURL string, oldEnoughMin int, requestsPerMinute int) *Client {
	// Calculate interval between requests
	interval := time.Minute / time.Duration(requestsPerMinute-1)

	log.Info().
		Int("requests_per_minute", requestsPerMinute).
		Dur("request_interval", interval).
		Str("base_url", baseURL).
		Int("old_enough_min", oldEnoughMin).
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
				// Successfully added a token
				log.Trace().Msg("Added token to request channel")
			default:
				// Buffer full, skip this token
				log.Trace().Msg("Request channel buffer full, skipping token")
			}
		}
	}()

	client := &Client{
		httpClient:    &http.Client{Timeout: time.Second * 30},
		apiKey:        apiKey,
		baseURL:       baseURL,
		OldEnoughMin:  oldEnoughMin,
		requestTicker: ticker,
		requestChan:   requestChan,
	}

	log.Info().Msg("PUBG API client initialized successfully")
	return client
}

// request is the internal method that makes an HTTP request
// with optional rate limiting
func (c *Client) request(endpoint string, shouldRateLimit bool) ([]byte, error) {
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	startTime := time.Now()

	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	log.Debug().
		Str("request_id", requestID).
		Str("endpoint", endpoint).
		Str("url", url).
		Bool("rate_limited", shouldRateLimit).
		Msg("Preparing API request")

	// Apply rate limiting if needed
	if shouldRateLimit {
		waitStart := time.Now()
		log.Debug().
			Str("request_id", requestID).
			Msg("Waiting for rate limit token")

		// Wait for permission to make a request
		<-c.requestChan

		waitDuration := time.Since(waitStart)
		log.Debug().
			Str("request_id", requestID).
			Dur("wait_duration", waitDuration).
			Msg("Acquired rate limit token")
	}

	// Create request with headers
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("url", url).
			Dur("prep_duration", time.Since(startTime)).
			Msg("Error creating request")
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/vnd.api+json")

	// Execute request
	execStart := time.Now()
	log.Debug().
		Str("request_id", requestID).
		Str("url", url).
		Dur("prep_duration", execStart.Sub(startTime)).
		Msg("Executing API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("url", url).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", time.Since(execStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error executing request")
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	readStart := time.Now()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("url", url).
			Int("status_code", resp.StatusCode).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", readStart.Sub(execStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error reading response body")
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseAPIError(resp.StatusCode, respBody)
		log.Error().
			Str("request_id", requestID).
			Err(apiErr).
			Str("url", url).
			Int("status_code", resp.StatusCode).
			Int("response_size", len(respBody)).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", readStart.Sub(execStart)).
			Dur("read_duration", time.Since(readStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("API returned error response")
		return nil, apiErr
	}

	// Success
	log.Debug().
		Str("request_id", requestID).
		Str("url", url).
		Int("status_code", resp.StatusCode).
		Int("response_size", len(respBody)).
		Dur("prep_duration", execStart.Sub(startTime)).
		Dur("exec_duration", readStart.Sub(execStart)).
		Dur("read_duration", time.Since(readStart)).
		Dur("total_duration", time.Since(startTime)).
		Msg("API request completed successfully")

	return respBody, nil
}

// Request is a backward-compatible function that always applies rate limiting
func (c *Client) Request(endpoint string) ([]byte, error) {
	log.Debug().
		Str("endpoint", endpoint).
		Msg("Using rate-limited request (legacy method)")
	return c.RequestRateLimited(endpoint)
}

// RequestRateLimited makes a rate-limited request to the PUBG API
func (c *Client) RequestRateLimited(endpoint string) ([]byte, error) {
	log.Debug().
		Str("endpoint", endpoint).
		Msg("Making rate-limited request")
	return c.request(endpoint, true)
}

// RequestNonRateLimited makes a request to the PUBG API without rate limiting
func (c *Client) RequestNonRateLimited(endpoint string) ([]byte, error) {
	log.Debug().
		Str("endpoint", endpoint).
		Msg("Making non-rate-limited request")
	return c.request(endpoint, false)
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

// GetTelemetry retrieves a telemetry file with optional rate limiting
func (c *Client) GetTelemetry(telemetryURL string, shouldRateLimit bool) ([]byte, error) {
	requestID := fmt.Sprintf("telemetry_%d", time.Now().UnixNano())
	startTime := time.Now()

	log.Debug().
		Str("request_id", requestID).
		Str("telemetry_url", telemetryURL).
		Bool("rate_limited", shouldRateLimit).
		Msg("Preparing telemetry request")

	// Apply rate limiting if needed
	if shouldRateLimit {
		waitStart := time.Now()
		log.Debug().
			Str("request_id", requestID).
			Msg("Waiting for rate limit token for telemetry request")

		// Wait for permission to make a request
		<-c.requestChan

		waitDuration := time.Since(waitStart)
		log.Debug().
			Str("request_id", requestID).
			Dur("wait_duration", waitDuration).
			Msg("Acquired rate limit token for telemetry request")
	}

	// Create the request
	req, err := http.NewRequest(http.MethodGet, telemetryURL, nil)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("telemetry_url", telemetryURL).
			Dur("prep_duration", time.Since(startTime)).
			Msg("Error creating telemetry request")
		return nil, fmt.Errorf("error creating telemetry request: %w", err)
	}

	// Set the authorization header with your bearer token
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	// Execute request
	execStart := time.Now()
	log.Debug().
		Str("request_id", requestID).
		Str("telemetry_url", telemetryURL).
		Dur("prep_duration", execStart.Sub(startTime)).
		Msg("Executing telemetry request")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("telemetry_url", telemetryURL).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", time.Since(execStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error executing telemetry request")
		return nil, fmt.Errorf("error making telemetry request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	readStart := time.Now()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Str("telemetry_url", telemetryURL).
			Int("status_code", resp.StatusCode).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", readStart.Sub(execStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Error reading telemetry response")
		return nil, fmt.Errorf("error reading telemetry response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := parseAPIError(resp.StatusCode, respBody)
		log.Error().
			Str("request_id", requestID).
			Err(apiErr).
			Str("telemetry_url", telemetryURL).
			Int("status_code", resp.StatusCode).
			Int("response_size", len(respBody)).
			Dur("prep_duration", execStart.Sub(startTime)).
			Dur("exec_duration", readStart.Sub(execStart)).
			Dur("read_duration", time.Since(readStart)).
			Dur("total_duration", time.Since(startTime)).
			Msg("Telemetry request returned error response")
		return nil, apiErr
	}

	// Success
	log.Debug().
		Str("request_id", requestID).
		Str("telemetry_url", telemetryURL).
		Int("status_code", resp.StatusCode).
		Int("response_size", len(respBody)).
		Dur("prep_duration", execStart.Sub(startTime)).
		Dur("exec_duration", readStart.Sub(execStart)).
		Dur("read_duration", time.Since(readStart)).
		Dur("total_duration", time.Since(startTime)).
		Msg("Telemetry request completed successfully")

	return respBody, nil
}

// GetTelemetryRateLimited retrieves a telemetry file with rate limiting
func (c *Client) GetTelemetryRateLimited(telemetryURL string) ([]byte, error) {
	log.Debug().
		Str("telemetry_url", telemetryURL).
		Msg("Making rate-limited telemetry request")
	return c.GetTelemetry(telemetryURL, true)
}

// GetTelemetryNonRateLimited retrieves a telemetry file without rate limiting
func (c *Client) GetTelemetryNonRateLimited(telemetryURL string) ([]byte, error) {
	log.Debug().
		Str("telemetry_url", telemetryURL).
		Msg("Making non-rate-limited telemetry request")
	return c.GetTelemetry(telemetryURL, false)
}

// Close stops the ticker when the client is no longer needed
func (c *Client) Close() {
	if c.requestTicker != nil {
		log.Info().Msg("Shutting down PUBG API client")
		c.requestTicker.Stop()
	}
}
