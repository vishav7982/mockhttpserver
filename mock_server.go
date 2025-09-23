package moxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Protocol:               HTTP,
		UnmatchedStatusCode:    http.StatusTeapot,
		UnmatchedStatusMessage: "Unmatched Request",
		LogUnmatched:           true,
		MaxBodySize:            10 << 20, // 10MB
		VerboseLogging:         false,
	}
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

	if config.Protocol == HTTPS {
		server := httptest.NewUnstartedServer(http.HandlerFunc(ms.handler))
		server.TLS = buildTLSConfig(config.TLSConfig)
		server.StartTLS()
		ms.server = server
	} else {
		ms.server = httptest.NewServer(http.HandlerFunc(ms.handler))
	}
	return ms
}

// buildTLSConfig builds a *tls.Config from TLSOptions.
func buildTLSConfig(opts *TLSOptions) *tls.Config {
	tlsConfig := &tls.Config{}

	if opts == nil {
		tlsConfig.Certificates = []tls.Certificate{generateDefaultCert()}
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig
	}
	if opts.MinVersion != 0 {
		tlsConfig.MinVersion = opts.MinVersion
	} else {
		tlsConfig.MinVersion = tls.VersionTLS12 // default
	}
	// Server certs
	if len(opts.Certificates) > 0 {
		tlsConfig.Certificates = opts.Certificates
	} else {
		tlsConfig.Certificates = []tls.Certificate{generateDefaultCert()}
	}
	// mTLS configuration
	if opts.RequireClientCert {
		if opts.SkipClientVerify {
			tlsConfig.ClientAuth = tls.RequireAnyClientCert
		} else {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
			tlsConfig.ClientCAs = opts.ClientCAs
		}
	}
	// Allow skipping verification (self-signed)
	tlsConfig.InsecureSkipVerify = opts.InsecureSkipVerify
	return tlsConfig
}

// WithLogger allows injecting a custom logger.
func (m *MockServer) WithLogger(logger *log.Logger) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
	return m
}

// WithUnmatchedResponder allows setting a custom handler for unmatched requests.
func (m *MockServer) WithUnmatchedResponder(
	handler func(w http.ResponseWriter, r *http.Request, req UnmatchedRequest),
) *MockServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unmatchedResponder = handler
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
func (m *MockServer) VerifyExpectations() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var unmet []string
	for _, exp := range m.expectations {
		if exp.MaxCalls != nil && exp.InvocationCount != *exp.MaxCalls {
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
func (m *MockServer) handler(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	if r.Body != nil {
		if m.config.MaxBodySize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, m.config.MaxBodySize)
		}
		body, err = io.ReadAll(r.Body)
		_ = r.Body.Close()
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
	for _, exp := range m.expectations {
		if exp.matches(r, body) {
			if exp.MaxCalls != nil && exp.InvocationCount >= *exp.MaxCalls {
				continue
			}
			exp.InvocationCount++
			resp := ResponseDefinition{}
			// If user configured responses, pick the right one
			if len(exp.Responses) > 0 {
				resp = exp.Responses[exp.NextResponseIndex]
				if exp.NextResponseIndex < len(exp.Responses)-1 {
					exp.NextResponseIndex++
				}
			}
			if resp.TimeoutSimulation {
				<-r.Context().Done() // blocks until the request is canceled by the client
				return
			}
			// Simulate delayed response.
			if resp.Delay > 0 {
				time.Sleep(resp.Delay)
			}
			// Write headers
			for key, value := range resp.Headers {
				w.Header().Set(key, value)
			}
			w.WriteHeader(resp.StatusCode)
			if _, err := w.Write(resp.Body); err != nil {
				m.logger.Printf("Failed to write response: %v", err)
			}
			if m.config.VerboseLogging {
				m.logger.Printf("Matched expectation, responding with status %d", resp.StatusCode)
			}
			return
		}
	}

	// No match -> record unmatched
	unmatched := UnmatchedRequest{
		Method:    r.Method,
		URL:       r.URL.RequestURI(),
		Headers:   map[string][]string(r.Header),
		Body:      string(body),
		Timestamp: time.Now(),
	}
	m.unmatchedRequests = append(m.unmatchedRequests, unmatched)

	if m.config.LogUnmatched {
		m.logger.Printf("Unexpected Request:\nMethod=%s\nURI=%s\nHeaders=%+v\nBody=%s\n",
			r.Method, r.URL.RequestURI(), r.Header, string(body))
	}

	if m.unmatchedResponder != nil {
		m.unmatchedResponder(w, r, unmatched)
		return
	}
	_ = fmt.Sprintf("Unmatched Request:\nMethod=%s\nURI=%s\nHeaders=%+v\nBody=%s\n", r.Method, r.URL.RequestURI(), r.Header, string(body))
	http.Error(w, m.config.UnmatchedStatusMessage, m.config.UnmatchedStatusCode)
}

// DefaultClient returns a simple *http.Client for HTTP/HTTPS testing.
// This client:
//   - Works for HTTP
//   - Works for HTTPS with server certs if InsecureSkipVerify is true
//   - DOES NOT handle mTLS; for that, create a custom client with TLS config
func (m *MockServer) DefaultClient() *http.Client {
	transport := &http.Transport{}
	if m.config.Protocol == HTTPS {
		// Simple HTTPS client
		tlsConfig := &tls.Config{}
		if m.config.TLSConfig != nil {
			// Default client should always skip verification for normal HTTPS
			// (unless explicitly required otherwise)
			tlsConfig.InsecureSkipVerify = true
		} else {
			tlsConfig.InsecureSkipVerify = true
		}
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Transport: transport}
}

// mTLSClient returns an *http.Client configured for mutual TLS.
// It uses the TLSOptions from the server. The caller provides client certificates and RootCAs.
// This is useful for testing mTLS scenarios where the server verifies the client certificate.
func (m *MockServer) mTLSClient(clientCerts []tls.Certificate, rootCAs *x509.CertPool) *http.Client {
	tlsConfig := &tls.Config{
		Certificates: clientCerts, // allow multiple client certs
		RootCAs:      rootCAs,     // trust server cert
		MinVersion:   tls.VersionTLS12,
	}

	// If the server requires client certs, ensure the client provides one
	if m.config.TLSConfig != nil && m.config.TLSConfig.RequireClientCert {
		tlsConfig.InsecureSkipVerify = false
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}

// Use adds middleware to the mock server (applied to all requests).
func (m *MockServer) Use(middleware func(http.Handler) http.Handler) {
	m.server.Config.Handler = middleware(m.server.Config.Handler)
}
func (e *ExpectationError) Error() string {
	result := e.Message
	for _, detail := range e.Details {
		result += "\n  " + detail
	}
	return result
}
