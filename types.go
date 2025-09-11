package mockhttpserver

import (
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"time"
)

// ResponseDefinition defines a mock response for an expectation.
type ResponseDefinition struct {
	StatusCode        int
	Body              []byte
	Headers           map[string]string
	Delay             time.Duration // optional delay before sending response
	TimeoutSimulation bool          // if true, server never responds
}

// RequestExpectation defines the expected request structure.
type RequestExpectation struct {
	Method        string
	Path          string
	PathPattern   *regexp.Regexp
	PathVariables map[string]string
	Body          []byte
	BodyMatcher   func([]byte) bool
	QueryParams   map[string]string
	Headers       map[string]string // stored as lowercase keys for case-insensitive matching
	BodyFromFile  bool
}

// Expectation defines a mock expectation for HTTP requests.
// It contains the expected request and one or more sequential responses.
type Expectation struct {
	Request             RequestExpectation
	Responses           []ResponseDefinition
	CreateResponseIndex int
	InvocationCount     int
	MaxCalls            *int // nil means unlimited
	NextResponseIndex   int  // tracks which response to return next
}

// MockServer represents a lightweight HTTP mock server for testing HTTP clients.
type MockServer struct {
	server             *httptest.Server
	expectations       []*Expectation
	unmatchedRequests  []UnmatchedRequest
	mu                 sync.RWMutex
	logger             *log.Logger
	config             Config
	unmatchedResponder func(w http.ResponseWriter, r *http.Request, req UnmatchedRequest)
}

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
	UnmatchedStatusCode    int    // Status code for unmatched requests (default: 418)
	UnmatchedStatusMessage string // Status message for unmatched requests (default: "Unmatched Request")
	LogUnmatched           bool   // Whether to log unmatched requests (default: true)
	MaxBodySize            int64  // Maximum request body size in bytes (default: 10MB)
	VerboseLogging         bool   // Enable verbose request/response logging (default: false)
}

// ExpectationError represents errors related to unmet expectations
type ExpectationError struct {
	Message string
	Details []string
}

// mockRoundTripper allows http.Client to route through the mock server.
type mockRoundTripper struct {
	server *MockServer
}
