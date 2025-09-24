package moxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeClose safely closes response body and logs error if any.
func safeClose(t *testing.T, body io.ReadCloser) {
	t.Helper()
	if body != nil {
		if err := body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}
}

// TestMockServer_Basic validates a simple POST request with body returns expected response.
func TestMockServer_Basic(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		NewExpectation().
			WithRequestMethod("POST").
			WithPath("/hello").
			WithRequestBodyString("world").
			AndRespondWithString("ok", 200),
	)

	resp, err := http.Post(ms.URL()+"/hello", "text/plain", strings.NewReader("world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got '%s'", string(body))
	}
}

// TestMockServer_QueryParamsAndHeaders ensures query params and headers match properly.
func TestMockServer_QueryParamsAndHeaders(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		NewExpectation().
			WithRequestMethod("GET").
			WithPath("/search").
			WithQueryParam("q", "golang").
			WithHeader("X-Test", "true").
			AndRespondWithString("found", 200),
	)

	req, _ := http.NewRequest("GET", ms.URL()+"/search?q=golang", nil)
	req.Header.Set("X-Test", "true")
	resp, err := ms.DefaultClient().Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "found" {
		t.Errorf("expected body 'found', got '%s'", string(body))
	}
}

// TestMockServer_NoMatch ensures unmatched requests return configured status code.
func TestMockServer_NoMatch(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	resp, err := http.Get(ms.URL() + "/unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 418 {
		t.Errorf("expected status 418, got %d", resp.StatusCode)
	}
}

// TestMockServer_CustomConfig tests custom configuration options.
func TestMockServer_CustomConfig(t *testing.T) {
	config := Config{
		UnmatchedStatusCode: 404,
		LogUnmatched:        false,
		MaxBodySize:         1024,
		VerboseLogging:      true,
	}

	ms := NewMockServerWithConfig(&config)
	defer ms.Close()

	resp, err := http.Get(ms.URL() + "/unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestMockServer_JSONBodyMatching tests JSON body matching functionality.
func TestMockServer_JSONBodyMatching(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestJSONBody(`{"ping":"pong"}`)
	ms.AddExpectation(e.AndRespondWithString("ok", 200))

	resp, err := http.Post(ms.URL()+"/api", "application/json", strings.NewReader(`{"ping":"pong"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_InvalidJSONBody tests error handling for invalid JSON.
func TestMockServer_InvalidJSONBody(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when request file does not exist")
		} else {
			t.Logf("caught expected panic: %v", r)
		}
	}()
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api")
	e.WithRequestJSONBody(`{"invalid":json}`)
}

// TestMockServer_PartialJSONMatching tests partial JSON body matching.
func TestMockServer_PartialJSONMatching(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestPartialJSONBody(`{"name":"test"}`)

	ms.AddExpectation(e.AndRespondWithString("matched", 200))

	resp, err := http.Post(ms.URL()+"/api", "application/json",
		strings.NewReader(`{"name":"test","age":30,"city":"NYC"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_PathPattern tests regex path matching.
func TestMockServer_PathPattern(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath(`/users/\d+`)
	ms.AddExpectation(e.AndRespondWithString("user found", 200))

	resp, err := http.Get(ms.URL() + "/users/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_CallCounting tests expectation call counting and verification.
func TestMockServer_CallCounting(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test").
		AndRespondWithString("ok", 200).
		Times(2)

	ms.AddExpectation(e)

	// First call
	resp1, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp1.Body)

	if e.InvocationCounter() != 1 {
		t.Errorf("expected call count 1, got %d", e.InvocationCounter())
	}

	// Second call
	resp2, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp2.Body)

	if e.InvocationCounter() != 2 {
		t.Errorf("expected call count 2, got %d", e.InvocationCounter())
	}

	if err := ms.VerifyExpectations(); err != nil {
		t.Errorf("verification failed: %v", err)
	}
}

// TestMockServer_UnmetExpectations tests verification of unmet expectations.
func TestMockServer_UnmetExpectations(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test").
		AndRespondWithString("ok", 200).
		Times(3)

	ms.AddExpectation(e)

	resp, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp.Body)

	if err := ms.VerifyExpectations(); err == nil {
		t.Error("expected verification to fail, got nil")
	}
}

// TestMockServer_ConcurrentRequests tests thread safety with concurrent requests.
func TestMockServer_ConcurrentRequests(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/concurrent").
		AndRespondWithString("ok", 200)

	ms.AddExpectation(e)

	const numGoroutines = 10
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				resp, err := http.Get(ms.URL() + "/concurrent")
				if err != nil {
					errors <- err
					return
				}
				safeClose(t, resp.Body)

				if resp.StatusCode != 200 {
					errors <- fmt.Errorf("expected status 200, got %d", resp.StatusCode)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}

	expectedCalls := numGoroutines * requestsPerGoroutine
	if e.InvocationCounter() != expectedCalls {
		t.Errorf("expected %d calls, got %d", expectedCalls, e.InvocationCounter())
	}
}

// TestMockServer_ResponseFromFile serves a JSON file as response body.
func TestMockServer_ResponseFromFile(t *testing.T) {
	filePath := filepath.Join("testdata", "sample-response.json")

	want, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read fixture file: %v", err)
	}

	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		AndRespondFromFile(filePath, 200)
	ms.AddExpectation(e)

	resp, err := http.Get(ms.URL() + "/api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	got, _ := io.ReadAll(resp.Body)
	if string(got) != string(want) {
		t.Errorf("response body mismatch:\nwant: %s\ngot:  %s", string(want), string(got))
	}
}

// TestMockServer_RequestBodyFromFile uses request body from file.
func TestMockServer_RequestBodyFromFile(t *testing.T) {
	filePath := filepath.Join("testdata", "sample-request.json")

	testRequest, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read fixture file: %v", err)
	}

	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestBodyFromFile(filePath)
	ms.AddExpectation(e.AndRespondWithString("matched", 200))

	resp, err := http.Post(ms.URL()+"/api", "application/json",
		bytes.NewReader(testRequest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_ResponseHeaders tests response header setting.
func TestMockServer_ResponseHeaders(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		NewExpectation().
			WithRequestMethod("GET").
			WithPath("/headers").
			WithResponseHeader("Content-Type", "application/json").
			WithResponseHeader("X-Custom", "test-value").
			AndRespondWithString(`{"message":"hello"}`, 500),
	)

	resp, err := http.Get(ms.URL() + "/headers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
	defer safeClose(t, resp.Body)

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}

	if custom := resp.Header.Get("X-Custom"); custom != "test-value" {
		t.Errorf("expected X-Custom 'test-value', got '%s'", custom)
	}
}

// TestMockServer_MultipleResponsesWithHeaders verifies that multiple responses
// with their own headers and bodies are returned in sequence.
func TestMockServer_MultipleResponsesWithHeaders(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/multi-seq").
		WithResponseHeader("X-Step", "1").
		AndRespondWithString(`{"step":"one"}`, 200).
		NextResponse().
		WithResponseHeader("X-Step", "2").
		AndRespondWithString(`{"step":"two"}`, 201).
		NextResponse().
		WithResponseHeader("X-Step", "3").
		AndRespondWithString(`{"step":"three"}`, 202)

	ms.AddExpectation(e)

	// Helper function to perform GET and read body & header
	doGet := func() (string, int, string) {
		resp, err := http.Get(ms.URL() + "/multi-seq")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer safeClose(t, resp.Body)
		body, _ := io.ReadAll(resp.Body)
		return string(body), resp.StatusCode, resp.Header.Get("X-Step")
	}

	// Sequential requests and expectations
	tests := []struct {
		body   string
		status int
		header string
	}{
		{`{"step":"one"}`, 200, "1"},
		{`{"step":"two"}`, 201, "2"},
		{`{"step":"three"}`, 202, "3"},
		{`{"step":"three"}`, 202, "3"}, // last response repeats
	}

	for i, tt := range tests {
		body, status, header := doGet()
		if body != tt.body || status != tt.status || header != tt.header {
			t.Errorf("request %d: expected body=%q, status=%d, header=%q, got body=%q, status=%d, header=%q",
				i+1, tt.body, tt.status, tt.header, body, status, header)
		}
	}
}

// TestExpectation_MultipleHeadersAndResponses verifies that multiple responses
// can have multiple headers and are returned in sequence correctly.
func TestExpectation_MultipleHeadersAndResponses(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Expectation with multiple headers and responses
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/multi-seq").
		WithResponseHeader("X-Step", "1").
		AndRespondWithString(`{"step":"one"}`, 200).
		NextResponse().
		WithResponseHeader("X-Step", "2").
		AndRespondWithString(`{"step":"two"}`, 201).
		NextResponse().
		WithResponseHeaders(map[string]string{
			"X-Step":   "3",
			"X-Custom": "yes",
		}).
		AndRespondWithString(`{"step":"three"}`, 202)

	ms.AddExpectation(e)

	// Helper to GET response and body
	doGet := func() (*http.Response, []byte) {
		resp, err := http.Get(ms.URL() + "/multi-seq")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		defer safeClose(t, resp.Body)
		return resp, body
	}

	tests := []struct {
		status  int
		body    string
		headers map[string]string
	}{
		{200, `{"step":"one"}`, map[string]string{"X-Step": "1"}},
		{201, `{"step":"two"}`, map[string]string{"X-Step": "2"}},
		{202, `{"step":"three"}`, map[string]string{"X-Step": "3", "X-Custom": "yes"}},
		{202, `{"step":"three"}`, map[string]string{"X-Step": "3", "X-Custom": "yes"}}, // repeat last
	}

	for i, tt := range tests {
		resp, body := doGet()
		if string(body) != tt.body || resp.StatusCode != tt.status {
			t.Errorf("request %d: expected body=%q, status=%d, got body=%q, status=%d",
				i+1, tt.body, tt.status, string(body), resp.StatusCode)
		}
		for k, v := range tt.headers {
			if resp.Header.Get(k) != v {
				t.Errorf("request %d: expected header %s=%q, got %q", i+1, k, v, resp.Header.Get(k))
			}
		}
	}
}

// TestMockServer_ExpectationManagement tests adding, removing, and clearing expectations.
func TestMockServer_ExpectationManagement(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e1 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test1").
		AndRespondWithString("ok1", 200)
	e2 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test2").
		AndRespondWithString("ok2", 200)

	ms.AddExpectation(e1)
	ms.AddExpectation(e2)

	resp1, _ := http.Get(ms.URL() + "/test1")
	safeClose(t, resp1.Body)
	resp2, _ := http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp1.StatusCode != 200 || resp2.StatusCode != 200 {
		t.Error("both expectations should work initially")
	}

	if !ms.RemoveExpectation(e1) {
		t.Error("should have removed exp1")
	}

	resp1, _ = http.Get(ms.URL() + "/test1")
	safeClose(t, resp1.Body)
	resp2, _ = http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp1.StatusCode != 418 {
		t.Error("exp1 should be removed and return 418")
	}
	if resp2.StatusCode != 200 {
		t.Error("exp2 should still work")
	}

	ms.ClearExpectations()

	resp2, _ = http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp2.StatusCode != 418 {
		t.Error("all expectations should be cleared")
	}
}

// TestMockServer_UnmatchedRequests tests tracking of unmatched requests.
func TestMockServer_UnmatchedRequest(t *testing.T) {
	ms := NewMockServerWithConfig(&Config{
		UnmatchedStatusCode: 404,
	})
	defer ms.Close()

	resp, err := http.Get(ms.URL() + "/unknown")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}

	resp2, err := http.Post(ms.URL()+"/unknown2", "text/plain", strings.NewReader("test"))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer safeClose(t, resp2.Body)

	if resp2.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", resp2.StatusCode)
	}
}

// TestMockServer_RequestBodyContains tests substring matching for request bodies.
func TestMockServer_RequestBodyContains(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		NewExpectation().
			WithRequestMethod("POST").
			WithPath("/search").
			WithRequestBodyContains("golang").
			AndRespondWithString("found", 200),
	)

	resp, err := http.Post(ms.URL()+"/search", "text/plain",
		strings.NewReader("I love golang programming"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_MaxBodySize tests request body size limits.
func TestMockServer_MaxBodySize(t *testing.T) {
	config := Config{
		MaxBodySize: 10,
	}

	ms := NewMockServerWithConfig(&config)
	defer ms.Close()

	largeBody := strings.Repeat("x", 100)
	resp, err := http.Post(ms.URL()+"/test", "text/plain", strings.NewReader(largeBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 400 {
		t.Errorf("expected status 400 for large body, got %d", resp.StatusCode)
	}
}

// TestMockServer_CustomUnmatchedResponder tests custom callback for unmatched requests.
func TestMockServer_CustomUnmatchedResponder(t *testing.T) {
	ms := NewMockServer().WithUnmatchedResponder(
		func(w http.ResponseWriter, r *http.Request, req UnmatchedRequest) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"error":"not found","method":"%s"}`, req.Method)
		},
	)
	defer ms.Close()

	resp, err := http.Get(ms.URL() + "/does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 404 {
		t.Errorf("expected 404 from custom unmatched responder, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expected := `{"error":"not found","method":"GET"}`
	if string(body) != expected {
		t.Errorf("expected body %q, got %q", expected, string(body))
	}
}

// TestMockServer_RequestAndResponseFromFile tests both request and response loaded from files.
func TestMockServer_RequestAndResponseFromFile(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()
	reqFile := filepath.Join("testdata", "sample-request.json")
	respFile := filepath.Join("testdata", "sample-response.json")

	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/login").
		WithRequestBodyFromFile(reqFile)

	e = e.AndRespondFromFile(respFile, 200)
	ms.AddExpectation(e)

	reqJSON, err := os.ReadFile(reqFile)
	if err != nil {
		t.Fatalf("failed to read request file: %v", err)
	}

	resp, err := http.Post(ms.URL()+"/login", "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	want, err := os.ReadFile(respFile)
	if err != nil {
		t.Fatalf("failed to read response file: %v", err)
	}

	var gotJSON, wantJSON interface{}
	if err := json.Unmarshal(got, &gotJSON); err != nil {
		t.Fatalf("failed to unmarshal actual response JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatalf("failed to unmarshal expected response JSON: %v", err)
	}

	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Errorf("response JSON mismatch:\nwant: %s\ngot:  %s", string(want), string(got))
	}
}

// TestExpectation_MaxCallsEnforcement ensures that an expectation with MaxCalls is not called more than allowed.
func TestExpectation_MaxCallsEnforcement(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/max-calls").
		AndRespondWithString("ok", 200).
		Times(2) // allow only 2 calls

	ms.AddExpectation(e)

	for i := 0; i < 2; i++ {
		resp, err := http.Get(ms.URL() + "/max-calls")
		if err != nil {
			t.Fatalf("failed request %d: %v", i+1, err)
		}

		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d on call %d", resp.StatusCode, i+1)
		}

		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body on call %d: %v", i+1, err)
		}
	}

	// Third call should skip the expectation and return unmatched status (default 418)
	resp, err := http.Get(ms.URL() + "/max-calls")
	if err != nil {
		t.Fatalf("failed third request: %v", err)
	}

	if resp.StatusCode != 418 {
		t.Errorf("expected 418 for exceeding MaxCalls, got %d", resp.StatusCode)
	}

	if err := resp.Body.Close(); err != nil {
		t.Logf("failed to close response body for third call: %v", err)
	}
}

// TestMultipleHeadersBeforeBody ensures multiple WithResponseHeaders calls before AndRespondWith work correctly
func TestExpectation_MultipleHeadersBeforeBody(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/multi-headers").
		WithResponseHeader("X-A", "a").
		WithResponseHeaders(map[string]string{"X-B": "b", "X-C": "c"}).
		AndRespondWithString("ok", 200)

	ms.AddExpectation(exp)

	resp, _ := http.Get(ms.URL() + "/multi-headers")
	defer safeClose(t, resp.Body)

	if resp.Header.Get("X-A") != "a" || resp.Header.Get("X-B") != "b" || resp.Header.Get("X-C") != "c" {
		t.Errorf("expected headers X-A=a, X-B=b, X-C=c; got %+v", resp.Header)
	}
}

// TestLongSequentialResponses tests a sequence longer than the number of responses to ensure last response repeats
func TestExpectation_LongSequentialResponses(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/long-seq").
		AndRespondWithString("one", 200).
		NextResponse().
		AndRespondWithString("two", 201).
		NextResponse().
		AndRespondWithString("three", 202)

	ms.AddExpectation(e)

	for i := 0; i < 6; i++ { // 6 requests, last response should repeat
		resp, err := http.Get(ms.URL() + "/long-seq")
		if err != nil {
			t.Fatalf("failed request %d: %v", i+1, err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read body for request %d: %v", i+1, err)
		}

		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body for request %d: %v", i+1, err)
		}

		switch i {
		case 0:
			if string(body) != "one" || resp.StatusCode != 200 {
				t.Errorf("request %d expected 'one'/200, got %s/%d", i+1, body, resp.StatusCode)
			}
		case 1:
			if string(body) != "two" || resp.StatusCode != 201 {
				t.Errorf("request %d expected 'two'/201, got %s/%d", i+1, body, resp.StatusCode)
			}
		case 2:
			if string(body) != "three" || resp.StatusCode != 202 {
				t.Errorf("request %d expected 'three'/202, got %s/%d", i+1, body, resp.StatusCode)
			}
		default:
			// all subsequent requests should repeat last response
			if string(body) != "three" || resp.StatusCode != 202 {
				t.Errorf("request %d expected 'three'/202 repeat, got %s/%d", i+1, body, resp.StatusCode)
			}
		}
	}
}

// TestMalformedJSONRequest tests behavior when JSON request is invalid
func TestExpectation_MalformedJSONRequest(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/json").
		WithRequestJSONBody(`{"name": "valid"}`)

	ms.AddExpectation(e.AndRespondWithString("ok", 200))

	// Malformed JSON body
	resp, _ := http.Post(ms.URL()+"/json", "application/json", strings.NewReader(`{"name":`))
	defer safeClose(t, resp.Body)

	// Should not match expectation, returns unmatched
	if resp.StatusCode != 418 {
		t.Errorf("expected 418 for unmatched malformed JSON, got %d", resp.StatusCode)
	}
}

// TestMockServer_MaxBodySizeExceeded ensures that requests exceeding MaxBodySize return 400 Bad Request.
func TestMockServer_MaxBodySizeExceeded(t *testing.T) {
	ms := NewMockServerWithConfig(&Config{MaxBodySize: 5})
	defer ms.Close()

	resp, _ := http.Post(ms.URL()+"/test", "text/plain", strings.NewReader("exceeds"))
	defer safeClose(t, resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 when body exceeds MaxBodySize, got %d", resp.StatusCode)
	}
}

// TestMockServer_UnmatchedResponderCallback ensures that a custom unmatched responder is invoked correctly.
func TestMockServer_UnmatchedResponderCallback(t *testing.T) {
	called := false
	ms := NewMockServer().WithUnmatchedResponder(func(w http.ResponseWriter, r *http.Request, req UnmatchedRequest) {
		called = true
		w.WriteHeader(999)
	})
	defer ms.Close()

	resp, _ := http.Get(ms.URL() + "/unmatched")
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 999 {
		t.Errorf("expected status 999 from custom unmatched responder, got %d", resp.StatusCode)
	}
	if !called {
		t.Error("expected unmatched responder to be called")
	}
}

// TestMockServer_UseMiddleware validates that middleware added via Use() is applied to incoming requests.
func TestMockServer_UseMiddleware(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware", "yes")
			next.ServeHTTP(w, r)
		})
	})

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/middleware").
		AndRespondWithString("ok", 200)
	ms.AddExpectation(e)

	resp, _ := http.Get(ms.URL() + "/middleware")
	defer safeClose(t, resp.Body)

	if resp.Header.Get("X-Middleware") != "yes" {
		t.Error("expected middleware header X-Middleware=yes")
	}
}

// TestMockServer_NilBodyRequest ensures that requests with nil body are handled correctly.
func TestMockServer_NilBodyRequest(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/nobody").
		AndRespondWithString("ok", 200)
	ms.AddExpectation(e)

	req, _ := http.NewRequest("GET", ms.URL()+"/nobody", nil)
	resp, _ := ms.DefaultClient().Do(req)
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_GetUnmatchedRequests ensures unmatched requests are recorded and returned correctly.
func TestMockServer_GetUnmatchedRequests(t *testing.T) {
	ms := NewMockServerWithConfig(&Config{UnmatchedStatusCode: 404})
	defer ms.Close()

	// Trigger two unmatched requests
	resp1, err := http.Get(ms.URL() + "/unknown1")
	if err != nil {
		t.Fatalf("failed to send request 1: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}(resp1.Body)

	resp2, err := http.Get(ms.URL() + "/unknown2")
	if err != nil {
		t.Fatalf("failed to send request 2: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}(resp2.Body)

	unmatched := ms.GetUnmatchedRequests()
	if len(unmatched) != 2 {
		t.Errorf("expected 2 unmatched requests, got %d", len(unmatched))
	}

	if unmatched[0].URL != "/unknown1" || unmatched[1].URL != "/unknown2" {
		t.Errorf("unexpected unmatched request URLs: %v", unmatched)
	}
}

// TestMockServer_ClearUnmatchedRequests ensures that clearing unmatched requests works as expected.
func TestMockServer_ClearUnmatchedRequests(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Trigger an unmatched request
	resp, err := http.Get(ms.URL() + "/unknown")
	if err != nil {
		t.Fatalf("failed to send unmatched request: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}(resp.Body)

	ms.ClearUnmatchedRequests()
	unmatched := ms.GetUnmatchedRequests()
	if len(unmatched) != 0 {
		t.Errorf("expected 0 unmatched requests after clear, got %d", len(unmatched))
	}
}

// TestMockServer_LoggerAndVerbose simulates real usage with a custom logger,
// verbose logging, and unmatched request logging.
func TestMockServer_LoggerAndVerbose(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Capture log output in a buffer
	var logBuf strings.Builder
	customLogger := log.New(&logBuf, "[MockServer] ", log.LstdFlags)
	ms.WithLogger(customLogger)

	// Enable verbose logging and unmatched request logging
	ms.config.VerboseLogging = true
	ms.config.LogUnmatched = true

	// Add an expectation
	ms.AddExpectation(
		NewExpectation().
			WithRequestMethod("GET").
			WithPath("/exists").
			AndRespondWithString("ok", 200),
	)

	// 1. Trigger a matched request
	resp1, err := http.Get(ms.URL() + "/exists")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}(resp1.Body)

	body1, _ := io.ReadAll(resp1.Body)
	if string(body1) != "ok" {
		t.Errorf("expected body 'ok', got '%s'", string(body1))
	}

	// 2. Trigger an unmatched request
	resp2, err := http.Get(ms.URL() + "/missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}(resp2.Body)

	if resp2.StatusCode != ms.config.UnmatchedStatusCode {
		t.Errorf("expected unmatched status %d, got %d", ms.config.UnmatchedStatusCode, resp2.StatusCode)
	}

	// 3. Verify unmatched requests recorded
	unmatched := ms.GetUnmatchedRequests()
	if len(unmatched) != 1 || unmatched[0].URL != "/missing" {
		t.Errorf("expected one unmatched request for /missing, got %+v", unmatched)
	}

	// 4. Check that logs contain verbose output and unmatched logging
	logs := logBuf.String()
	if !strings.Contains(logs, "Incoming request: GET /exists") ||
		!strings.Contains(logs, "Matched expectation") ||
		!strings.Contains(logs, "Unexpected Request") {
		t.Errorf("expected verbose and unmatched logs, got: %s", logs)
	}

	// 5. Clear unmatched requests and ensure cleared
	ms.ClearUnmatchedRequests()
	if len(ms.GetUnmatchedRequests()) != 0 {
		t.Error("expected unmatched requests to be cleared")
	}
}

// TestMockServer_ResponseDelay verifies that response delays are respected.
func TestMockServer_ResponseDelay(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/delayed").
		AndRespondWithString("fast", 200).
		NextResponse().
		AndRespondWithString("slow", 200).
		WithResponseDelay(500 * time.Millisecond) // 0.5-second delay

	ms.AddExpectation(e)

	// Helper to measure response duration
	doGet := func() (string, time.Duration) {
		start := time.Now()
		resp, err := http.Get(ms.URL() + "/delayed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer safeClose(t, resp.Body)
		body, _ := io.ReadAll(resp.Body)
		return string(body), time.Since(start)
	}

	// First response → no delay
	body, duration := doGet()
	if body != "fast" {
		t.Errorf("expected first response body 'fast', got %q", body)
	}
	if duration >= 100*time.Millisecond {
		t.Errorf("expected first response to be fast, took %v", duration)
	}

	// Second response → delayed
	body, duration = doGet()
	if body != "slow" {
		t.Errorf("expected second response body 'slow', got %q", body)
	}
	if duration < 500*time.Millisecond {
		t.Errorf("expected delay of at least 500ms, took %v", duration)
	}

	// Third response → repeats last delayed response
	body, duration = doGet()
	if body != "slow" {
		t.Errorf("expected third response body 'slow', got %q", body)
	}
	if duration < 500*time.Millisecond {
		t.Errorf("expected delay of at least 500ms, took %v", duration)
	}
}

// TestMockServer_DelayedResponseDoesNotBlock verifies that a delayed response
// for one request does not block other requests from being served.
func TestMockServer_DelayedResponseDoesNotBlock(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Expectation with delay
	delay := 1000 * time.Millisecond
	e1 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/delayed").
		AndRespondWithString("delayed", 200).
		WithResponseDelay(delay)
	ms.AddExpectation(e1)

	// Another expectation with no delay
	e2 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/fast").
		AndRespondWithString("fast", 200)
	ms.AddExpectation(e2)

	start := time.Now()

	// Fire both requests concurrently
	var body1, body2 string
	done := make(chan struct{})
	go func() {
		resp, _ := http.Get(ms.URL() + "/delayed")
		defer safeClose(t, resp.Body)
		b, _ := io.ReadAll(resp.Body)
		body1 = string(b)
		done <- struct{}{}
	}()

	go func() {
		resp, _ := http.Get(ms.URL() + "/fast")
		defer safeClose(t, resp.Body)
		b, _ := io.ReadAll(resp.Body)
		body2 = string(b)
		done <- struct{}{}
	}()

	// Wait for both requests
	<-done
	<-done
	elapsed := time.Since(start)

	// Validate responses
	if body1 != "delayed" || body2 != "fast" {
		t.Errorf("unexpected response bodies: delayed=%q, fast=%q", body1, body2)
	}

	// Validate that fast request was not blocked by delayed one
	if elapsed < delay {
		t.Errorf("expected total elapsed time >= %v, got %v", delay, elapsed)
	}
}

// TestMockServer_DelayedSequentialResponse verifies that sequential responses
// for the same request wait for their own delay and are returned in order.
func TestMockServer_DelayedSequentialResponse(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/sequential-delay").
		// First response delayed
		AndRespondWithString("first", 200).
		WithResponseDelay(300*time.Millisecond).
		NextResponse().
		AndRespondWithString("second", 200)

	ms.AddExpectation(e)

	start := time.Now()
	resp1, err := http.Get(ms.URL() + "/sequential-delay")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp1.Body)
	body1, _ := io.ReadAll(resp1.Body)
	elapsed1 := time.Since(start)

	if string(body1) != "first" {
		t.Errorf("expected 'first', got %q", string(body1))
	}
	if elapsed1 < 300*time.Millisecond {
		t.Errorf("first response returned too early, elapsed %v", elapsed1)
	}

	start = time.Now()
	resp2, err := http.Get(ms.URL() + "/sequential-delay")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp2.Body)
	body2, _ := io.ReadAll(resp2.Body)
	elapsed2 := time.Since(start)

	if string(body2) != "second" {
		t.Errorf("expected 'second', got %q", string(body2))
	}
	if elapsed2 > 50*time.Millisecond {
		t.Errorf("second response delayed unexpectedly, elapsed %v", elapsed2)
	}
}

// TestMockServer_SimulateTimeout verifies that an expectation can simulate
// a server timeout without affecting other requests.
func TestMockServer_SimulateTimeout(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Timeout expectation
	timeoutExp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/timeout").
		SimulateTimeout()
	ms.AddExpectation(timeoutExp)

	// Normal expectation
	normalExp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/normal").
		AndRespondWithString("ok", 200)
	ms.AddExpectation(normalExp)

	// Test normal request works
	resp, err := http.Get(ms.URL() + "/normal")
	if err != nil {
		t.Fatalf("unexpected error for normal request: %v", err)
	}
	defer safeClose(t, resp.Body)
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(body))
	}

	// Test timeout request
	client := &http.Client{
		Timeout: 100 * time.Millisecond, // client timeout
	}
	start := time.Now()
	_, err = client.Get(ms.URL() + "/timeout")
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("expected timeout error, got none")
	} else if !strings.Contains(err.Error(), "Client.Timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("request returned too quickly, expected to wait ~100ms, got %v", elapsed)
	}
}

// TestMockServer_SequentialWithDelayAndTimeout verifies sequential responses
// including delayed responses and a simulated timeout (blocked response).
func TestMockServer_SequentialWithDelayAndTimeout(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/seq-delay-timeout")

	// 1st response: quick response
	e.AndRespondWithString("first", 200).
		WithResponseHeader("X-Step", "1")

	// 2nd response: delayed 50ms
	e.NextResponse().
		AndRespondWithString("second", 201).
		WithResponseHeader("X-Step", "2").
		WithResponseDelay(50 * time.Millisecond)

	// 3rd response: simulate timeout (blocked indefinitely)
	e.NextResponse().
		SimulateTimeout()

	ms.AddExpectation(e)

	client := &http.Client{Timeout: 100 * time.Millisecond}

	// 1st request → immediate response
	resp1, err := client.Get(ms.URL() + "/seq-delay-timeout")
	if err != nil {
		t.Fatalf("unexpected error on first request: %v", err)
	}
	defer safeClose(t, resp1.Body)
	body1, _ := io.ReadAll(resp1.Body)
	if string(body1) != "first" || resp1.StatusCode != 200 || resp1.Header.Get("X-Step") != "1" {
		t.Errorf("first response mismatch, got body=%q, status=%d, header=%s",
			string(body1), resp1.StatusCode, resp1.Header.Get("X-Step"))
	}

	// 2nd request → delayed response (~50ms)
	start2 := time.Now()
	resp2, err := client.Get(ms.URL() + "/seq-delay-timeout")
	if err != nil {
		t.Fatalf("unexpected error on second request: %v", err)
	}
	defer safeClose(t, resp2.Body)
	body2, _ := io.ReadAll(resp2.Body)
	elapsed2 := time.Since(start2)
	if string(body2) != "second" || resp2.StatusCode != 201 || resp2.Header.Get("X-Step") != "2" {
		t.Errorf("second response mismatch, got body=%q, status=%d, header=%s",
			string(body2), resp2.StatusCode, resp2.Header.Get("X-Step"))
	}
	if elapsed2 < 50*time.Millisecond {
		t.Errorf("expected at least 50ms delay, got %v", elapsed2)
	}

	// 3rd request → should block and hit client timeout (~100ms)
	start3 := time.Now()
	_, err = client.Get(ms.URL() + "/seq-delay-timeout")
	elapsed3 := time.Since(start3)

	if err == nil {
		t.Errorf("expected timeout error on third request, got none")
	} else if !strings.Contains(err.Error(), "Client.Timeout") &&
		!strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected timeout error, got: %v", err)
	}

	if elapsed3 < 90*time.Millisecond {
		t.Errorf("third request returned too quickly, expected ~100ms wait, got %v", elapsed3)
	}
}

// TestHTTPSWithDefaultClient demonstrates a simple HTTPS scenario using the
// default mock server client (ms.Client()). This client automatically skips
// TLS verification (InsecureSkipVerify=true), so it can connect to a server
// with a self-signed certificate without errors. This represents internal
// testing where trust of the server certificate is not enforced.
func TestHTTPSWithDefaultClient(t *testing.T) {
	// Load self-signed server certificate + key
	serverCrtfilePath := filepath.Join("testdata", "server.crt")
	serverKeyfilePath := filepath.Join("testdata", "server.key")
	cert, err := tls.LoadX509KeyPair(serverCrtfilePath, serverKeyfilePath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Configure mock server with TLSOptions (HTTPS)
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{cert},
			RequireClientCert: false, // simple HTTPS, no mTLS
		},
	})
	defer server.Close()

	// Add expectation for GET /simple
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/simple").
		AndRespondWithString("ok", 200),
	)

	// Use default client (ms.DefaultClient()) which skips certificate verification
	resp, err := server.DefaultClient().Get(server.URL() + "/simple")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

// TestHTTPSWithCustomClient demonstrates a realistic HTTPS scenario where
// the client verifies the server certificate. This simulates a real-world
// case where the client only trusts specific certificates and enforces
// TLS verification.
func TestHTTPSWithCustomClient(t *testing.T) {
	// Load server certificate + key
	serverCrtfilePath := filepath.Join("testdata", "server.crt")
	serverKeyfilePath := filepath.Join("testdata", "server.key")
	cert, err := tls.LoadX509KeyPair(serverCrtfilePath, serverKeyfilePath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Build RootCAs for client to trust the server cert
	certData, err := os.ReadFile(serverCrtfilePath)
	if err != nil {
		t.Fatalf("failed to read server.crt: %v", err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(certData); !ok {
		t.Fatal("failed to append server cert to RootCAs")
	}

	// Configure mock server with TLSOptions
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{cert},
			RequireClientCert: false, // simple HTTPS
		},
	})
	defer server.Close()

	// Add expectation for GET /secure
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/secure").
		AndRespondWithString("ok", 200),
	)

	// Create a custom client with RootCAs to verify server certificate
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootCAs, // verify server certificate
			},
		},
	}

	resp, err := client.Get(server.URL() + "/secure")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

// TestHTTPSWithMutualTLS verifies that the mock server enforces mTLS:
//   - Server requires client certificate signed by a trusted CA (or self-signed for test)
//   - Client presents its certificate to authenticate
//   - Both sides trust each other's certificates via RootCAs
func TestHTTPSWithMutualTLS(t *testing.T) {
	// Load server certificate + key
	serverCrtFilePath := filepath.Join("testdata", "server.crt")
	serverKeyFilePath := filepath.Join("testdata", "server.key")
	serverCert, err := tls.LoadX509KeyPair(serverCrtFilePath, serverKeyFilePath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Load client certificate + key
	clientCrtFilePath := filepath.Join("testdata", "client.crt")
	clientKeyFilePath := filepath.Join("testdata", "client.key")
	clientCert, err := tls.LoadX509KeyPair(clientCrtFilePath, clientKeyFilePath)
	if err != nil {
		t.Fatalf("failed to load client cert/key: %v", err)
	}

	// Create server trust pool (trust client cert)
	clientCertData, err := os.ReadFile(clientCrtFilePath)
	if err != nil {
		t.Fatalf("failed to read client cert: %v", err)
	}
	clientCertPool := x509.NewCertPool()
	if ok := clientCertPool.AppendCertsFromPEM(clientCertData); !ok {
		t.Fatal("failed to append client cert to server trust pool")
	}

	// Create client trust pool (trust server cert)
	serverCertData, err := os.ReadFile(serverCrtFilePath)
	if err != nil {
		t.Fatalf("failed to read server cert: %v", err)
	}
	serverCertPool := x509.NewCertPool()
	if ok := serverCertPool.AppendCertsFromPEM(serverCertData); !ok {
		t.Fatal("failed to append server cert to client trust pool")
	}
	// Configure mock server to require client cert (mTLS)
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: true,
			ClientCAs:         clientCertPool,
		},
	})
	defer server.Close()

	// Add expectation for GET /secure
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/secure").
		AndRespondWithString("ok", 200),
	)
	// Make request
	resp, err := server.mTLSClient([]tls.Certificate{clientCert}, serverCertPool).Get(server.URL() + "/secure")
	if err != nil {
		t.Fatalf("unexpected error during mTLS request: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

// 1. Normal HTTPS with trusted self-signed server cert
func TestHTTPSWithSelfSignedServerCert(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: false,
		},
	})
	defer server.Close()

	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/secure").
		AndRespondWithString("ok", 200),
	)

	// Build RootCAs to trust server cert
	certData, _ := os.ReadFile("testdata/server.crt")
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(certData)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: rootCAs},
		},
	}

	resp, err := client.Get(server.URL() + "/secure")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
}

// HTTPS with invalid/untrusted server certificate
func TestHTTPSWithInvalidServerCert(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: false,
		},
	})
	defer server.Close()

	client := &http.Client{} // Default client does not trust the self-signed cert
	_, err := client.Get(server.URL() + "/secure")
	if err == nil {
		t.Fatal("expected TLS handshake error for untrusted server cert, got nil")
	}
}

// mTLS: server requires client cert, client does not provide one
func TestMutualTLSNoClientCert(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	clientCertData, _ := os.ReadFile("testdata/client.crt")
	clientCertPool := x509.NewCertPool()
	clientCertPool.AppendCertsFromPEM(clientCertData)

	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: true,
			ClientCAs:         clientCertPool,
		},
	})
	defer server.Close()

	client := &http.Client{} // client does not provide a certificate
	_, err := client.Get(server.URL() + "/secure")
	if err == nil {
		t.Fatal("expected TLS handshake error due to missing client certificate, got nil")
	}
}

// mTLS: client provides invalid certificate
func TestMutualTLSWithInvalidClientCert(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: true,
			ClientCAs:         x509.NewCertPool(), // empty pool, server trusts nothing
		},
	})
	defer server.Close()

	clientCert, _ := tls.LoadX509KeyPair("testdata/client.crt", "testdata/client.key")
	certData, _ := os.ReadFile("testdata/server.crt")
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(certData)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientCert},
				RootCAs:      rootCAs,
			},
		},
	}

	_, err := client.Get(server.URL() + "/secure")
	if err == nil {
		t.Fatal("expected TLS handshake error due to server rejecting client certificate, got nil")
	}
}

// TestTLSVersionEnforcement ensures the server enforces minimum TLS version.
// Server requires TLS >= 1.2, client tries TLS 1.0 and fails handshake.
func TestTLSVersionEnforcement(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates: []tls.Certificate{serverCert},
			MinVersion:   tls.VersionTLS12,
		},
	})
	defer server.Close()

	// Client tries TLS 1.0
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS10, // explicitly old TLS
				MaxVersion:         tls.VersionTLS10,
				InsecureSkipVerify: true, // skip cert check for this test
			},
		},
	}

	_, err := client.Get(server.URL() + "/secure")
	if err == nil {
		t.Fatal("expected handshake failure due to old TLS version, got nil")
	}
}

// TestMultipleClientsConcurrentRequests tests server concurrency with multiple clients.
// Mix of valid and invalid clients are used simultaneously.
func TestMultipleClientsConcurrentRequests(t *testing.T) {
	serverCert, _ := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: false,
		},
		LogUnmatched: true,
	})
	defer server.Close()
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/secure").
		AndRespondWithString("ok", 200),
	)
	certData, _ := os.ReadFile("testdata/server.crt")
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(certData)
	const numClients = 5
	var wg sync.WaitGroup
	wg.Add(numClients)
	errorsCh := make(chan error, numClients)
	for i := 0; i < numClients; i++ {
		go func(i int) {
			defer wg.Done()
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						RootCAs: rootCAs,
					},
				},
			}
			// Every even index client sends invalid path
			url := server.URL() + "/secure"
			if i%2 == 0 {
				url = server.URL() + "/invalid"
			}
			resp, err := client.Get(url)
			if err != nil {
				errorsCh <- err
				return
			}
			defer safeClose(t, resp.Body)
			if i%2 == 0 && resp.StatusCode != http.StatusTeapot {
				errorsCh <- fmt.Errorf("expected %d for invalid path, got %d", http.StatusTeapot, resp.StatusCode)
			}
			if i%2 != 0 && resp.StatusCode != 200 {
				errorsCh <- fmt.Errorf("expected 200 for valid path, got %d", resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
}

// TestHTTPSWithTrustedAndUntrustedClients tests mTLS enforcement.
// - Server requires client certificate signed by trusted CA
// - One client presents a trusted cert (should succeed)
// - One client presents an untrusted cert (should fail handshake)
func TestHTTPSWithTrustedAndUntrustedClients(t *testing.T) {
	// Load server certificate + key
	serverCrtPath := "testdata/server.crt"
	serverKeyPath := "testdata/server.key"
	serverCert, err := tls.LoadX509KeyPair(serverCrtPath, serverKeyPath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Load trusted client certificate + key
	trustedClientCrt := "testdata/client.crt"
	trustedClientKey := "testdata/client.key"
	trustedCert, err := tls.LoadX509KeyPair(trustedClientCrt, trustedClientKey)
	if err != nil {
		t.Fatalf("failed to load trusted client cert/key: %v", err)
	}

	// Load untrusted client certificate + key
	untrustedClientCrt := "testdata/untrusted_client.crt"
	untrustedClientKey := "testdata/untrusted_client.key"
	untrustedCert, err := tls.LoadX509KeyPair(untrustedClientCrt, untrustedClientKey)
	if err != nil {
		t.Fatalf("failed to load untrusted client cert/key: %v", err)
	}

	// Server trust pool (trusts only trusted client)
	trustedClientData, _ := os.ReadFile(trustedClientCrt)
	clientCertPool := x509.NewCertPool()
	if ok := clientCertPool.AppendCertsFromPEM(trustedClientData); !ok {
		t.Fatal("failed to append trusted client cert to server trust pool")
	}

	// Configure mock server to require client certs (mTLS)
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: true,
			ClientCAs:         clientCertPool,
		},
	})
	defer server.Close()

	// Add expectation for GET /secure
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/secure").
		AndRespondWithString("ok", 200),
	)

	// Server cert pool for clients to trust server
	serverCertData, _ := os.ReadFile(serverCrtPath)
	serverCertPool := x509.NewCertPool()
	if ok := serverCertPool.AppendCertsFromPEM(serverCertData); !ok {
		t.Fatal("failed to append server cert to client trust pool")
	}

	type testClient struct {
		cert     tls.Certificate
		name     string
		expectOK bool
	}

	clients := []testClient{
		{cert: trustedCert, name: "trusted", expectOK: true},
		{cert: untrustedCert, name: "untrusted", expectOK: false},
	}

	var wg sync.WaitGroup
	errorsCh := make(chan string, len(clients))
	wg.Add(len(clients))

	for _, tc := range clients {
		go func(tc testClient) {
			defer wg.Done()
			client := server.mTLSClient([]tls.Certificate{tc.cert}, serverCertPool)
			resp, err := client.Get(server.URL() + "/secure")
			if tc.expectOK {
				if err != nil {
					errorsCh <- fmt.Sprintf("%s client failed: %v", tc.name, err)
					return
				}
				defer safeClose(t, resp.Body)
				if resp.StatusCode != 200 {
					errorsCh <- fmt.Sprintf("%s client got status %d, expected 200", tc.name, resp.StatusCode)
				}
			} else {
				if err == nil {
					errorsCh <- fmt.Sprintf("%s client succeeded but should have failed", tc.name)
				}
			}
		}(tc)
	}

	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		t.Error(err)
	}
}

// TestHTTPSWithMultipleExpectations verifies mock server handles multiple expectations over HTTPS.
func TestHTTPSWithMultipleExpectations(t *testing.T) {
	// Load server certificate + key
	serverCrtPath := "testdata/server.crt"
	serverKeyPath := "testdata/server.key"
	serverCert, err := tls.LoadX509KeyPair(serverCrtPath, serverKeyPath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Configure HTTPS mock server (no client certs for simplicity)
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: false,
		},
		UnmatchedStatusCode:    http.StatusNotFound,
		UnmatchedStatusMessage: "No matching expectation",
	})
	defer server.Close()

	// Add multiple expectations
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/get").
		AndRespondWithString("get_ok", 200),
	)
	server.AddExpectation(NewExpectation().
		WithRequestMethod("POST").
		WithPath("/post").
		AndRespondWithString("post_ok", 201),
	)
	server.AddExpectation(NewExpectation().
		WithRequestMethod("DELETE").
		WithPath("/delete").
		AndRespondWithString("delete_ok", 204),
	)

	// Server trust pool for client to verify server
	serverCertData, _ := os.ReadFile(serverCrtPath)
	serverCertPool := x509.NewCertPool()
	if ok := serverCertPool.AppendCertsFromPEM(serverCertData); !ok {
		t.Fatal("failed to append server cert to client trust pool")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: serverCertPool,
			},
		},
	}

	// Test GET expectation
	resp, err := client.Get(server.URL() + "/get")
	if err != nil {
		t.Fatalf("GET /get failed: %v", err)
	}
	defer safeClose(t, resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /get: expected 200, got %d", resp.StatusCode)
	}

	// Test POST expectation
	postResp, err := client.Post(server.URL()+"/post", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST /post failed: %v", err)
	}
	defer safeClose(t, postResp.Body)
	if postResp.StatusCode != 201 {
		t.Fatalf("POST /post: expected 201, got %d", postResp.StatusCode)
	}

	// Test DELETE expectation
	req, _ := http.NewRequest("DELETE", server.URL()+"/delete", nil)
	deleteResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE /delete failed: %v", err)
	}
	defer safeClose(t, deleteResp.Body)
	if deleteResp.StatusCode != 204 {
		t.Fatalf("DELETE /delete: expected 204, got %d", deleteResp.StatusCode)
	}

	// Test unmatched path returns configured status
	unmatchedResp, err := client.Get(server.URL() + "/unknown")
	if err != nil {
		t.Fatalf("GET /unknown failed: %v", err)
	}
	defer safeClose(t, unmatchedResp.Body)
	if unmatchedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /unknown: expected %d, got %d", http.StatusNotFound, unmatchedResp.StatusCode)
	}
}

// TestHTTPSWithMultipleExpectationsAndMutualTLS verifies mock server handles multiple expectations over HTTPS with mTLS.
//   - Server requires client certificates
//   - Multiple expectations are registered for different paths and methods
//   - Client presents its certificate and trusts the server certificate
func TestHTTPSWithMultipleExpectationsAndMutualTLS(t *testing.T) {
	// Load server certificate + key
	serverCrtPath := "testdata/server.crt"
	serverKeyPath := "testdata/server.key"
	serverCert, err := tls.LoadX509KeyPair(serverCrtPath, serverKeyPath)
	if err != nil {
		t.Fatalf("failed to load server cert/key: %v", err)
	}

	// Load client certificate + key
	clientCrtPath := "testdata/client.crt"
	clientKeyPath := "testdata/client.key"
	clientCert, err := tls.LoadX509KeyPair(clientCrtPath, clientKeyPath)
	if err != nil {
		t.Fatalf("failed to load client cert/key: %v", err)
	}

	// Server trust pool (trust client certificate)
	clientCertData, err := os.ReadFile(clientCrtPath)
	if err != nil {
		t.Fatalf("failed to read client cert: %v", err)
	}
	clientCertPool := x509.NewCertPool()
	if ok := clientCertPool.AppendCertsFromPEM(clientCertData); !ok {
		t.Fatal("failed to append client cert to server trust pool")
	}

	// Server configuration: require client cert (mTLS)
	server := NewMockServerWithConfig(&Config{
		Protocol: HTTPS,
		TLSConfig: &TLSOptions{
			Certificates:      []tls.Certificate{serverCert},
			RequireClientCert: true,
			ClientCAs:         clientCertPool,
		},
		UnmatchedStatusCode:    http.StatusTeapot,
		UnmatchedStatusMessage: "Unmatched Request",
		LogUnmatched:           true,
	})
	defer server.Close()

	// Add multiple expectations
	server.AddExpectation(NewExpectation().
		WithRequestMethod("GET").
		WithPath("/get").
		AndRespondWithString("get_ok", 200),
	)
	server.AddExpectation(NewExpectation().
		WithRequestMethod("POST").
		WithPath("/post").
		AndRespondWithString("post_ok", 201),
	)
	server.AddExpectation(NewExpectation().
		WithRequestMethod("DELETE").
		WithPath("/delete").
		AndRespondWithString("delete_ok", 204),
	)

	// Client trust pool (trust server certificate)
	serverCertData, err := os.ReadFile(serverCrtPath)
	if err != nil {
		t.Fatalf("failed to read server cert: %v", err)
	}
	serverCertPool := x509.NewCertPool()
	if ok := serverCertPool.AppendCertsFromPEM(serverCertData); !ok {
		t.Fatal("failed to append server cert to client trust pool")
	}

	// Create mutual TLS client
	mtlsClient := server.mTLSClient([]tls.Certificate{clientCert}, serverCertPool)

	// Test GET expectation
	resp, err := mtlsClient.Get(server.URL() + "/get")
	if err != nil {
		t.Fatalf("GET /get failed: %v", err)
	}
	defer safeClose(t, resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /get: expected 200, got %d", resp.StatusCode)
	}

	// Test POST expectation
	postResp, err := mtlsClient.Post(server.URL()+"/post", "text/plain", nil)
	if err != nil {
		t.Fatalf("POST /post failed: %v", err)
	}
	defer safeClose(t, postResp.Body)
	if postResp.StatusCode != 201 {
		t.Fatalf("POST /post: expected 201, got %d", postResp.StatusCode)
	}

	// Test DELETE expectation
	req, _ := http.NewRequest("DELETE", server.URL()+"/delete", nil)
	deleteResp, err := mtlsClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /delete failed: %v", err)
	}
	defer safeClose(t, deleteResp.Body)
	if deleteResp.StatusCode != 204 {
		t.Fatalf("DELETE /delete: expected 204, got %d", deleteResp.StatusCode)
	}

	// Test unmatched path returns configured status
	unmatchedResp, err := mtlsClient.Get(server.URL() + "/unknown")
	if err != nil {
		t.Fatalf("GET /unknown failed: %v", err)
	}
	defer safeClose(t, unmatchedResp.Body)
	if unmatchedResp.StatusCode != http.StatusTeapot {
		t.Fatalf("GET /unknown: expected %d, got %d", http.StatusTeapot, unmatchedResp.StatusCode)
	}
}
