package mockserver

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
)

// MockServer represents a lightweight HTTP mock server that can
// be used to simulate responses for testing HTTP clients.
type MockServer struct {
	server       *httptest.Server
	expectations []*Expectation
	mu           sync.Mutex
	logger       *log.Logger
}

// NewMockServer initializes a new MockServer with default logger.
func NewMockServer() *MockServer {
	ms := &MockServer{
		logger: log.New(os.Stdout, "[MockServer] ", log.LstdFlags|log.Lshortfile),
	}
	ms.server = httptest.NewServer(http.HandlerFunc(ms.handler))
	return ms
}

// WithLogger allows injecting a custom logger (e.g., zap, slog adapter).
func (m *MockServer) WithLogger(logger *log.Logger) {
	m.logger = logger
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

// handler processes incoming HTTP requests and returns the configured mock response.
// It supports matching on method, path, query parameters, headers, and request body.
// If no expectation matches, it responds with HTTP 418 "I'm a Teapot".
func (m *MockServer) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.logger.Printf("Failed to read request body: %v", err)
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			m.logger.Printf("Failed to close request body: %v", cerr)
		}
	}()
	// Iterate over all expectations to find a match
outerLoop:
	for _, exp := range m.expectations {
		// Match HTTP method and path
		if r.Method != exp.Method || r.URL.Path != exp.Path {
			continue
		}
		// Match query parameters
		if len(exp.QueryParams) > 0 {
			q := r.URL.Query()
			for key, expectedValue := range exp.QueryParams {
				if q.Get(key) != expectedValue {
					continue outerLoop // Skip to next expectation if query param doesn't match
				}
			}
		}
		// Match headers
		if len(exp.Headers) > 0 {
			for key, expectedValue := range exp.Headers {
				if r.Header.Get(key) != expectedValue {
					continue outerLoop // Skip to next expectation if header doesn't match
				}
			}
		}
		// Match body if a matcher is defined
		if exp.bodyMatcher != nil {
			if !exp.bodyMatcher(body) {
				continue
			}
		} else if exp.ReqBody != "" && exp.ReqBody != string(body) {
			continue
		}
		// If all matchers pass, return configured response
		w.WriteHeader(exp.ResCode)
		if _, err := w.Write([]byte(exp.ResBody)); err != nil {
			m.logger.Printf("Failed to write response: %v", err)
		}
		return
	}

	// Log unexpected requests for debugging
	m.logger.Printf("Unexpected Request:\nMethod=%s\nURI=%s\nHeaders=%+v\nBody=%s\n",
		r.Method, r.URL.RequestURI(), r.Header, string(body))

	// Respond with 418 "I'm a Teapot" for unmatched requests
	http.Error(w, "Unexpected request", http.StatusTeapot)
}

// Client returns an *http.Client that uses this mock server.
func (m *MockServer) Client() *http.Client {
	return &http.Client{
		Transport: &mockRoundTripper{server: m},
	}
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
