package safehttp

import (
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// ErrorRecord holds information about errors and retries for a time window.
type ErrorRecord struct {
	TotalRequests  int
	FailedRequests int
	Retries        int
	Timestamp      time.Time
}

// SafeClient is an HTTP client with enhanced safety features like circuit breaking.
type SafeClient struct {
	httpClient     Doer
	ErrorThreshold float64
	RetryThreshold float64
	WindowDuration time.Duration
	ErrorRecords   []ErrorRecord
	mu             sync.Mutex
	Logger         *slog.Logger
}

// New creates a new instance of SafeClient with configurable settings.
func New(client Doer, errorThreshold, retryThreshold float64, windowDuration time.Duration) *SafeClient {
	return &SafeClient{
		httpClient:     client,
		ErrorThreshold: errorThreshold,
		RetryThreshold: retryThreshold,
		WindowDuration: windowDuration,
		Logger:         slog.Default(),
	}
}

// SendRequest sends an HTTP request with retry logic and circuit breaker checks.
func (c *SafeClient) SendRequest(req *http.Request) (*http.Response, error) {
	maxRetries := 3
	retryCount := 0

	for {
		if c.ShouldBreakCircuit() {
			c.Logger.Warn("circuit breaker triggered, request aborted")
			return nil, errors.New("circuit breaker triggered, request aborted")
		}

		resp, err := c.httpClient.Do(req)
		if err == nil {
			c.UpdateErrorRecord(false, false)
			return resp, nil
		}

		if retryCount >= maxRetries {
			c.Logger.Warn("max retries exceeded, request aborted")
			c.UpdateErrorRecord(true, false)
			return nil, err
		}

		retryCount++
		c.UpdateErrorRecord(true, true)
		time.Sleep(c.CalculateBackoffTime(retryCount))
	}
}

// ShouldBreakCircuit checks if the circuit breaker conditions are met.
func (c *SafeClient) ShouldBreakCircuit() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	errorRate, retryRate := c.CalculateCurrentRates()
	c.Logger.Debug("rates",
		slog.Group("error",
			slog.Float64("rate", errorRate),
		),
		slog.Group("retry",
			slog.Float64("rate", retryRate),
		),
	)
	return errorRate > c.ErrorThreshold || retryRate > c.RetryThreshold
}

// CalculateBackoffTime calculates the time to wait before the next retry.
func (c *SafeClient) CalculateBackoffTime(retryCount int) time.Duration {
	baseDelay := 500 * time.Millisecond
	maxJitter := 100 * time.Millisecond

	backoff := time.Duration(retryCount) * baseDelay
	jitter := time.Duration(rand.Intn(int(maxJitter)))

	c.Logger.Debug("backoff time", slog.Any("time", backoff+jitter))
	return backoff + jitter
}

// UpdateErrorRecord updates the error record for the current window.
func (c *SafeClient) UpdateErrorRecord(isError, isRetry bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// Create a new record if the latest one is outdated
	if len(c.ErrorRecords) == 0 || c.ErrorRecords[len(c.ErrorRecords)-1].Timestamp.Before(now.Add(-c.WindowDuration)) {
		c.ErrorRecords = append(c.ErrorRecords, ErrorRecord{Timestamp: now})
	}

	currentRecord := &c.ErrorRecords[len(c.ErrorRecords)-1]
	currentRecord.TotalRequests++
	if isError {
		c.Logger.Error("request failed")
		currentRecord.FailedRequests++
	}
	if isRetry {
		c.Logger.Warn("retrying request")
		currentRecord.Retries++
	}

	c.CleanupOldRecords()
}

// CleanupOldRecords removes error records that are outside the time window.
func (c *SafeClient) CleanupOldRecords() {
	thresholdTime := time.Now().Add(-c.WindowDuration)
	newStart := 0
	for i, record := range c.ErrorRecords {
		if record.Timestamp.After(thresholdTime) {
			newStart = i
			break
		}
	}
	c.Logger.Debug("cleaning up old record", slog.Int("new start", newStart))
	c.ErrorRecords = c.ErrorRecords[newStart:]
}

// CalculateCurrentRates calculates the current error rate and retry rate.
func (c *SafeClient) CalculateCurrentRates() (float64, float64) {
	var totalRequests, totalFailed, totalRetries int
	for _, record := range c.ErrorRecords {
		totalRequests += record.TotalRequests
		totalFailed += record.FailedRequests
		totalRetries += record.Retries
	}

	if totalRequests == 0 {
		return 0, 0
	}

	errorRate := float64(totalFailed) / float64(totalRequests)
	retryRate := float64(totalRetries) / float64(totalRequests)
	c.Logger.Debug("current rates",
		slog.Group("error",
			slog.Float64("rate", errorRate),
			slog.Int("failed", totalFailed),
			slog.Int("total", totalRequests),
		),
		slog.Group("retry",
			slog.Float64("rate", retryRate),
			slog.Int("retries", totalRetries),
			slog.Int("total", totalRequests),
		),
	)
	return errorRate, retryRate
}
