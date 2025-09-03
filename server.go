package mockhttpserver

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"
)

// UnmatchedRequest represents a request that didn't match any expectations
type UnmatchedRequest struct {
	Method    string
	URL       string
	Headers   map[string][]string
	Body      string
	Timestamp time.Time
}

// Config holds configuration options for MockServer
type Config struct {
	UnmatchedStatusCode int   // Status code for unmatched requests (default: 418)
	LogUnmatched        bool  // Whether to log unmatched requests (default: true)
	MaxBodySize         int64 // Maximum request body size in bytes (default: 10MB)
	VerboseLogging      bool  // Enable verbose request/response logging (default: false)
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		UnmatchedStatusCode: http.StatusTeapot,
		LogUnmatched:        true,
		MaxBodySize:         10 << 20, // 10MB
		VerboseLogging:      false,
	}
}

// MockServer represents a lightweight HTTP mock server that can
// be used to simulate responses for testing HTTP clients.
type MockServer struct {
	server            *httptest.Server
	expectations      []*Expectation
	unmatchedRequests []UnmatchedRequest
	mu                sync.RWMutex
	logger            *log.Logger
	config            Config
}

// NewMockServer initializes a new MockServer with default configuration and logger.
func NewMockServer() *MockServer {
	return NewMockServerWithConfig(DefaultConfig())
}

// NewMockServerWithConfig initializes a new MockServer with custom configuration.
func NewMockServerWithConfig(config Config) *MockServer {
	ms := &MockServer{
		logger: log.New(os.Stdout, "[MockServer] ", log.LstdFlags|log.Lshortfile),
		config: config,
	}
	ms.server = httptest.NewServer(http.HandlerFunc(ms.handler))
	return ms
}

// WithLogger allows injecting a custom logger (e.g., zap, slog adapter).
func (m *MockServer) WithLogger(logger *log.Logger) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
	return m
}

// Close shuts down the mock server.
func (m *MockServer) Close() {
	m.server.Close()
}

// URL returns the base URL of the mock server.
func (m *MockServer) URL() string {
	return m.server.URL
}

// AddExpectation registers an expectation against which requests are matched.
func (m *MockServer) AddExpectation(e *Expectation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectations = append(m.expectations, e)
}

// ClearExpectations removes all registered expectations.
func (m *MockServer) ClearExpectations() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expectations = m.expectations[:0]
}

// RemoveExpectation removes a specific expectation. Returns true if found and removed.
func (m *MockServer) RemoveExpectation(e *Expectation) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, exp := range m.expectations {
		if exp == e {
			m.expectations = append(m.expectations[:i], m.expectations[i+1:]...)
			return true
		}
	}
	return false
}

// GetUnmatchedRequests returns a copy of all unmatched requests.
func (m *MockServer) GetUnmatchedRequests() []UnmatchedRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]UnmatchedRequest, len(m.unmatchedRequests))
	copy(result, m.unmatchedRequests)
	return result
}

// ClearUnmatchedRequests clears the history of unmatched requests.
func (m *MockServer) ClearUnmatchedRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unmatchedRequests = m.unmatchedRequests[:0]
}

// VerifyExpectations checks if all expectations were called the expected number of times.
// Returns an error describing any unmet expectations.
func (m *MockServer) VerifyExpectations() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var unmet []string
	for _, exp := range m.expectations {
		if exp.expectedCalls != nil && exp.callCount != *exp.expectedCalls {
			unmet = append(unmet, exp.String())
		}
	}

	if len(unmet) > 0 {
		return &ExpectationError{
			Message: "Unmet expectations found",
			Details: unmet,
		}
	}
	return nil
}

// handler processes incoming HTTP requests and returns the configured mock response.
// It supports matching on method, path, query parameters, headers, and request body.
// If no expectation matches, it responds with the configured unmatched status code.
func (m *MockServer) handler(w http.ResponseWriter, r *http.Request) {
	// Read request body with size limit
	var body []byte
	var err error

	if r.Body != nil {
		if m.config.MaxBodySize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxBodySize)
		}
		body, err = io.ReadAll(r.Body)
		r.Body.Close() // Close immediately after reading
		if err != nil {
			m.logger.Printf("Failed to read request body: %v", err)
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
	}

	if m.config.VerboseLogging {
		m.logger.Printf("Incoming request: %s %s, Headers: %+v, Body: %s",
			r.Method, r.URL.String(), r.Header, string(body))
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Iterate over all expectations to find a match
	for _, exp := range m.expectations {
		if exp.matches(r, body) {
			// Increment call count
			exp.callCount++

			// Set response headers if any
			for key, value := range exp.responseHeaders {
				w.Header().Set(key, value)
			}

			// Write response
			w.WriteHeader(exp.ResCode)
			if _, err := w.Write([]byte(exp.ResBody)); err != nil {
				m.logger.Printf("Failed to write response: %v", err)
			}

			if m.config.VerboseLogging {
				m.logger.Printf("Matched expectation, responding with status %d", exp.ResCode)
			}
			return
		}
	}

	// Record unmatched request
	unmatched := UnmatchedRequest{
		Method:    r.Method,
		URL:       r.URL.RequestURI(),
		Headers:   map[string][]string(r.Header),
		Body:      string(body),
		Timestamp: time.Now(),
	}
	m.unmatchedRequests = append(m.unmatchedRequests, unmatched)

	// Log unexpected requests for debugging
	if m.config.LogUnmatched {
		m.logger.Printf("Unexpected Request:\nMethod=%s\nURI=%s\nHeaders=%+v\nBody=%s\n",
			r.Method, r.URL.RequestURI(), r.Header, string(body))
	}

	// Respond with configured unmatched status code
	http.Error(w, "Unexpected request", m.config.UnmatchedStatusCode)
}

// Client returns an *http.Client that uses this mock server.
func (m *MockServer) Client() *http.Client {
	return &http.Client{
		Transport: &mockRoundTripper{server: m},
	}
}

// Use adds middleware to the mock server (applied to all requests).
func (m *MockServer) Use(middleware func(http.Handler) http.Handler) {
	m.server.Config.Handler = middleware(m.server.Config.Handler)
}

// mockRoundTripper allows http.Client to route through the mock server.
type mockRoundTripper struct {
	server *MockServer
}

func (rt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the request URL with the mock server URL
	req.URL.Scheme = "http"
	req.URL.Host = rt.server.server.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

// ExpectationError represents errors related to unmet expectations
type ExpectationError struct {
	Message string
	Details []string
}

func (e *ExpectationError) Error() string {
	result := e.Message
	for _, detail := range e.Details {
		result += "\n  " + detail
	}
	return result
}
