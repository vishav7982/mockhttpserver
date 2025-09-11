package mockhttpserver

import (
	"log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
)

// ResponseDefinition defines a mock response for an expectation.
type ResponseDefinition struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
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
	Request           RequestExpectation
	Responses         []ResponseDefinition
	InvocationCount   int
	MaxCalls          *int // nil means unlimited
	NextResponseIndex int  // tracks which response to return next
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
