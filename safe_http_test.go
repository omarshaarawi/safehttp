package safehttp_test

import (
	"errors"
	"github.com/omarshaarawi/safehttp"
	"net/http"
	"testing"
	"time"
)

type mockHTTPClient struct {
	responses []*http.Response
	errors    []error
}

func (m *mockHTTPClient) Do(_ *http.Request) (*http.Response, error) {
	if len(m.responses) == 0 {
		return &http.Response{StatusCode: http.StatusOK}, nil
	}
	resp, resps := m.responses[0], m.responses[1:]
	m.responses = resps
	err, errs := m.errors[0], m.errors[1:]
	m.errors = errs
	return resp, err
}

func TestSafeClientInitialization(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)

	if client.ErrorThreshold != 0.5 || client.RetryThreshold != 0.5 || client.WindowDuration != 1*time.Minute {
		t.Errorf("SafeClient properties are incorrect")
	}
}

func TestUpdateErrorRecord(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	initialTimestamp := time.Now()

	client.UpdateErrorRecord(true, false)
	client.UpdateErrorRecord(false, false)
	client.UpdateErrorRecord(true, true)

	if len(client.ErrorRecords) != 1 {
		t.Errorf("Expected 1 error record, got %d", len(client.ErrorRecords))
	}

	record := client.ErrorRecords[0]
	if record.Timestamp.Before(initialTimestamp) {
		t.Errorf("Record timestamp is incorrect")
	}
	if record.TotalRequests != 3 || record.FailedRequests != 2 || record.Retries != 1 {
		t.Errorf("Record counts are incorrect")
	}
}

func TestErrorRecordUpdateWithNoRetries(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)

	client.UpdateErrorRecord(true, false)

	record := client.ErrorRecords[0]
	if record.TotalRequests != 1 || record.FailedRequests != 1 || record.Retries != 0 {
		t.Errorf("record counts are incorrect")
	}
}

func TestErrorRecordUpdateWithRetries(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)

	client.UpdateErrorRecord(true, true)

	record := client.ErrorRecords[0]
	if record.TotalRequests != 1 || record.FailedRequests != 1 || record.Retries != 1 {
		t.Errorf("record counts are incorrect")
	}
}

func TestCalculateBackoffTime(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	retryCount := 2
	expectedMinBackoff := 1000 * time.Millisecond
	expectedMaxBackoff := 1100 * time.Millisecond

	backoffTime := client.CalculateBackoffTime(retryCount)
	if backoffTime < expectedMinBackoff || backoffTime > expectedMaxBackoff {
		t.Errorf("Backoff time %v is outside expected range [%v, %v]", backoffTime, expectedMinBackoff, expectedMaxBackoff)
	}
}

func TestBackoffTimeCalculationWithMaxRetries(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	backoffTime := client.CalculateBackoffTime(3)

	if backoffTime < 1500*time.Millisecond || backoffTime > 1600*time.Millisecond {
		t.Errorf("Backoff time %v is outside expected range [%v, %v]", backoffTime, 1500*time.Millisecond, 1600*time.Millisecond)
	}
}

func TestCircuitBreakerDecision(t *testing.T) {
	testCases := []struct {
		name             string
		errorThreshold   float64
		retryThreshold   float64
		windowDuration   time.Duration
		errorRecords     []safehttp.ErrorRecord
		expectedDecision bool
	}{
		{
			name:           "Circuit remains closed when error and retry rates are below thresholds",
			errorThreshold: 0.5,
			retryThreshold: 0.5,
			windowDuration: 1 * time.Minute,
			errorRecords: []safehttp.ErrorRecord{
				{TotalRequests: 100, FailedRequests: 40, Retries: 40, Timestamp: time.Now()},
			},
			expectedDecision: false,
		},
		{
			name:           "Circuit breaks when error rate is above threshold",
			errorThreshold: 0.5,
			retryThreshold: 0.5,
			windowDuration: 1 * time.Minute,
			errorRecords: []safehttp.ErrorRecord{
				{TotalRequests: 100, FailedRequests: 60, Retries: 40, Timestamp: time.Now()},
			},
			expectedDecision: true,
		},
		{
			name:           "Circuit breaks when retry rate is above threshold",
			errorThreshold: 0.5,
			retryThreshold: 0.5,
			windowDuration: 1 * time.Minute,
			errorRecords: []safehttp.ErrorRecord{
				{TotalRequests: 100, FailedRequests: 40, Retries: 60, Timestamp: time.Now()},
			},
			expectedDecision: true,
		},
		{
			name:           "Circuit breaks when both error and retry rates are above thresholds",
			errorThreshold: 0.5,
			retryThreshold: 0.5,
			windowDuration: 1 * time.Minute,
			errorRecords: []safehttp.ErrorRecord{
				{TotalRequests: 100, FailedRequests: 60, Retries: 60, Timestamp: time.Now()},
			},
			expectedDecision: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := safehttp.New(nil, tc.errorThreshold, tc.retryThreshold, tc.windowDuration)
			client.ErrorRecords = tc.errorRecords

			decision := client.ShouldBreakCircuit()
			if decision != tc.expectedDecision {
				t.Errorf("Expected decision %v, got %v", tc.expectedDecision, decision)
			}
		})
	}
}

func TestCurrentRateCalculation(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	client.ErrorRecords = []safehttp.ErrorRecord{
		{TotalRequests: 100, FailedRequests: 40, Retries: 60, Timestamp: time.Now()},
	}

	errorRate, retryRate := client.CalculateCurrentRates()

	if errorRate != 0.4 {
		t.Errorf("expected error rate to be 0.4, got %f", errorRate)
	}
	if retryRate != 0.6 {
		t.Errorf("expected retry rate to be 0.6, got %f", retryRate)
	}
}

func TestRateCalculationWithNoRequests(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)

	errorRate, retryRate := client.CalculateCurrentRates()

	if errorRate != 0 {
		t.Errorf("expected error rate to be 0, got %f", errorRate)
	}
	if retryRate != 0 {
		t.Errorf("expected retry rate to be 0, got %f", retryRate)
	}
}

func TestOutdatedRecordCleanup(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	client.ErrorRecords = []safehttp.ErrorRecord{
		{Timestamp: time.Now().Add(-2 * time.Minute)},
		{Timestamp: time.Now().Add(-1 * time.Minute)},
		{Timestamp: time.Now()},
	}

	client.CleanupOldRecords()

	if len(client.ErrorRecords) != 1 {
		t.Errorf("expected 1 error record, got %d", len(client.ErrorRecords))
	}
}

func TestRecordCleanupWithNoOutdatedRecords(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	client.ErrorRecords = []safehttp.ErrorRecord{
		{Timestamp: time.Now().Add(-30 * time.Second)},
		{Timestamp: time.Now()},
	}

	client.CleanupOldRecords()

	if len(client.ErrorRecords) != 2 {
		t.Errorf("expected 2 error records, got %d", len(client.ErrorRecords))
	}
}

func TestCircuitBreakerTriggeredByRetryRate(t *testing.T) {
	client := safehttp.New(nil, 0.5, 0.5, 1*time.Minute)
	client.RetryThreshold = 0.5
	client.ErrorRecords = []safehttp.ErrorRecord{
		{TotalRequests: 10, FailedRequests: 2, Retries: 6, Timestamp: time.Now()},
	}

	if !client.ShouldBreakCircuit() {
		t.Error("expected circuit breaker to be triggered, but it was not")
	}
}

func TestSendRequestSuccess(t *testing.T) {
	mockClient := &mockHTTPClient{
		responses: []*http.Response{{StatusCode: http.StatusOK}},
		errors:    []error{nil},
	}
	client := safehttp.New(mockClient, 0.5, 0.5, 1*time.Minute)

	req, _ := http.NewRequest("GET", "http:example.com", nil)
	resp, err := client.SendRequest(req)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp.StatusCode)
	}
}

func TestRetryLogicWithServerResponse(t *testing.T) {
	responses := []*http.Response{
		{StatusCode: http.StatusInternalServerError},
		{StatusCode: http.StatusInternalServerError},
		{StatusCode: http.StatusOK},
	}
	mockClient := &mockHTTPClient{
		responses: responses,
		errors:    make([]error, len(responses)),
	}
	client := safehttp.New(mockClient, 0.5, 0.5, 1*time.Minute)

	req, _ := http.NewRequest("GET", "http:example.com", nil)
	_, err := client.SendRequest(req)

	if err != nil {
		t.Error("expected no error, got error")
	}
}

func TestRequestSendWithRetriesOnFailure(t *testing.T) {
	mockClient := &mockHTTPClient{
		responses: []*http.Response{
			nil,
			nil,
			{StatusCode: http.StatusOK},
		},
		errors: []error{
			errors.New("mock error"),
			errors.New("mock error"),
			nil,
		},
	}

	client := safehttp.New(mockClient, 1.0, 1.0, 1*time.Minute)

	req, _ := http.NewRequest("GET", "http:example.com", nil)
	_, err := client.SendRequest(req)

	if err != nil {
		t.Errorf("expected no error, got error: %v", err)
	}

	lastRecord := client.ErrorRecords[len(client.ErrorRecords)-1]
	if lastRecord.FailedRequests != 2 || lastRecord.Retries != 2 {
		t.Errorf("UpdateErrorRecord was not called with the expected arguments. Got FailedRequests: %d, Retries: %d", lastRecord.FailedRequests, lastRecord.Retries)
	}
}

func TestRequestSendWithCircuitBreakerTriggered(t *testing.T) {
	mockClient := &mockHTTPClient{
		responses: []*http.Response{{StatusCode: http.StatusOK}},
		errors:    []error{nil},
	}

	client := safehttp.New(mockClient, 0.1, 0.5, 1*time.Minute)

	client.ErrorRecords = []safehttp.ErrorRecord{
		{TotalRequests: 10, FailedRequests: 2, Retries: 0, Timestamp: time.Now()},
	}

	req, _ := http.NewRequest("GET", "http:example.com", nil)
	_, err := client.SendRequest(req)

	if err == nil || err.Error() != "circuit breaker triggered, request aborted" {
		t.Error("expected circuit breaker error, got nil or wrong error")
	}

	if len(mockClient.responses) != 1 {
		t.Error("expected no HTTP requests to be made, but some were made")
	}
}

func TestRequestSendWithMaxRetriesReached(t *testing.T) {
	mockClient := &mockHTTPClient{
		responses: make([]*http.Response, 4),
		errors:    []error{errors.New("mock error"), errors.New("mock error"), errors.New("mock error"), errors.New("mock error")},
	}

	client := safehttp.New(mockClient, 1.0, 1.0, 1*time.Minute)

	req, _ := http.NewRequest("GET", "http:example.com", nil)
	_, err := client.SendRequest(req)

	if err == nil {
		t.Error("expected error, got nil")
	}

	lastRecord := client.ErrorRecords[len(client.ErrorRecords)-1]
	if lastRecord.FailedRequests != 4 || lastRecord.Retries != 3 {
		t.Error("UpdateErrorRecord was not called with the expected arguments")
	}
}
