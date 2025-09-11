package mockhttpserver

import (
	"bytes"
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
	resp, err := ms.Client().Do(req)
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

	ms := NewMockServerWithConfig(config)
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

	exp, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestJSONBody(`{"ping":"pong"}`)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

	ms.AddExpectation(exp.AndRespondWithString("ok", 200))

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
	_, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestJSONBody(`{"invalid":json}`)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestMockServer_PartialJSONMatching tests partial JSON body matching.
func TestMockServer_PartialJSONMatching(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithPartialJSONBody(`{"name":"test"}`)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

	ms.AddExpectation(exp.AndRespondWithString("matched", 200))

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath(`/users/\d+`)
	ms.AddExpectation(exp.AndRespondWithString("user found", 200))

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test").
		AndRespondWithString("ok", 200).
		Times(2)

	ms.AddExpectation(exp)

	// First call
	resp1, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp1.Body)

	if exp.InvocationCounter() != 1 {
		t.Errorf("expected call count 1, got %d", exp.InvocationCounter())
	}

	// Second call
	resp2, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp2.Body)

	if exp.InvocationCounter() != 2 {
		t.Errorf("expected call count 2, got %d", exp.InvocationCounter())
	}

	if err := ms.VerifyExpectations(); err != nil {
		t.Errorf("verification failed: %v", err)
	}
}

// TestMockServer_UnmetExpectations tests verification of unmet expectations.
func TestMockServer_UnmetExpectations(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test").
		AndRespondWithString("ok", 200).
		Times(3)

	ms.AddExpectation(exp)

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/concurrent").
		AndRespondWithString("ok", 200)

	ms.AddExpectation(exp)

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
	if exp.InvocationCounter() != expectedCalls {
		t.Errorf("expected %d calls, got %d", expectedCalls, exp.InvocationCounter())
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

	exp, err := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		AndRespondFromFile(filePath, 200)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}
	ms.AddExpectation(exp)

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

	exp, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestBodyFromFile(filePath)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}
	ms.AddExpectation(exp.AndRespondWithString("matched", 200))

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/multi-seq").
		WithResponseHeader("X-Step", "1").
		AndRespondWithString(`{"step":"one"}`, 200).
		WithResponseHeader("X-Step", "2").
		AndRespondWithString(`{"step":"two"}`, 201).
		WithResponseHeader("X-Step", "3").
		AndRespondWithString(`{"step":"three"}`, 202)

	ms.AddExpectation(exp)

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
	fmt.Println(ms.expectations)
	// 1st request → first response
	body, status, header := doGet()
	if body != `{"step":"one"}` || status != 200 || header != "1" {
		t.Errorf("expected first response with header 1, got body=%q, status=%d, header=%s", body, status, header)
	}

	// 2nd request → second response
	body, status, header = doGet()
	if body != `{"step":"two"}` || status != 201 || header != "2" {
		t.Errorf("expected second response with header 2, got body=%q, status=%d, header=%s", body, status, header)
	}

	// 3rd request → third response
	body, status, header = doGet()
	if body != `{"step":"three"}` || status != 202 || header != "3" {
		t.Errorf("expected third response with header 3, got body=%q, status=%d, header=%s", body, status, header)
	}

	// 4th request → should repeat last response
	body, status, header = doGet()
	if body != `{"step":"three"}` || status != 202 || header != "3" {
		t.Errorf("expected last response to repeat with header 3, got body=%q, status=%d, header=%s", body, status, header)
	}
}
func TestExpectation_MultipleHeadersAndResponses(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Expectation with multiple headers and responses
	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/multi-seq").
		WithResponseHeader("X-Step", "1").
		AndRespondWithString(`{"step":"one"}`, 200).
		WithResponseHeader("X-Step", "2").
		AndRespondWithString(`{"step":"two"}`, 201).
		WithResponseHeaders(map[string]string{
			"X-Step":   "3",
			"X-Custom": "yes",
		}).
		AndRespondWithString(`{"step":"three"}`, 202)

	ms.AddExpectation(exp)

	// First request
	resp1, _ := http.Get(ms.URL() + "/multi-seq")
	defer safeClose(t, resp1.Body)
	body1, _ := io.ReadAll(resp1.Body)
	if string(body1) != `{"step":"one"}` || resp1.StatusCode != 200 || resp1.Header.Get("X-Step") != "1" {
		t.Errorf("first response mismatch, got body=%s, status=%d, header=%s", string(body1), resp1.StatusCode, resp1.Header.Get("X-Step"))
	}

	// Second request
	resp2, _ := http.Get(ms.URL() + "/multi-seq")
	defer safeClose(t, resp2.Body)
	body2, _ := io.ReadAll(resp2.Body)
	if string(body2) != `{"step":"two"}` || resp2.StatusCode != 201 || resp2.Header.Get("X-Step") != "2" {
		t.Errorf("second response mismatch, got body=%s, status=%d, header=%s", string(body2), resp2.StatusCode, resp2.Header.Get("X-Step"))
	}

	// Third request
	resp3, _ := http.Get(ms.URL() + "/multi-seq")
	defer safeClose(t, resp3.Body)
	body3, _ := io.ReadAll(resp3.Body)
	if string(body3) != `{"step":"three"}` || resp3.StatusCode != 202 || resp3.Header.Get("X-Step") != "3" || resp3.Header.Get("X-Custom") != "yes" {
		t.Errorf("third response mismatch, got body=%s, status=%d, headers=%v", string(body3), resp3.StatusCode, resp3.Header)
	}

	// Fourth request (repeat last response)
	resp4, _ := http.Get(ms.URL() + "/multi-seq")
	defer safeClose(t, resp4.Body)
	body4, _ := io.ReadAll(resp4.Body)
	if string(body4) != `{"step":"three"}` || resp4.StatusCode != 202 || resp4.Header.Get("X-Step") != "3" || resp4.Header.Get("X-Custom") != "yes" {
		t.Errorf("fourth response mismatch, got body=%s, status=%d, headers=%v", string(body4), resp4.StatusCode, resp4.Header)
	}
}

// TestMockServer_ExpectationManagement tests adding, removing, and clearing expectations.
func TestMockServer_ExpectationManagement(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp1 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test1").
		AndRespondWithString("ok1", 200)
	exp2 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/test2").
		AndRespondWithString("ok2", 200)

	ms.AddExpectation(exp1)
	ms.AddExpectation(exp2)

	resp1, _ := http.Get(ms.URL() + "/test1")
	safeClose(t, resp1.Body)
	resp2, _ := http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp1.StatusCode != 200 || resp2.StatusCode != 200 {
		t.Error("both expectations should work initially")
	}

	if !ms.RemoveExpectation(exp1) {
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
	ms := NewMockServerWithConfig(Config{
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

	ms := NewMockServerWithConfig(config)
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

	exp, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/login").
		WithRequestBodyFromFile(reqFile)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

	exp, err = exp.AndRespondFromFile(respFile, 200)
	if err != nil {
		t.Fatalf("failed to attach response: %v", err)
	}

	ms.AddExpectation(exp)

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/max-calls").
		AndRespondWithString("ok", 200).
		Times(2) // allow only 2 calls

	ms.AddExpectation(exp)

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/long-seq").
		AndRespondWithString("one", 200).
		AndRespondWithString("two", 201).
		AndRespondWithString("three", 202)

	ms.AddExpectation(exp)

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

	exp, err := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/json").
		WithRequestJSONBody(`{"name": "valid"}`)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}
	ms.AddExpectation(exp.AndRespondWithString("ok", 200))

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
	ms := NewMockServerWithConfig(Config{MaxBodySize: 5})
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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/middleware").
		AndRespondWithString("ok", 200)
	ms.AddExpectation(exp)

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

	exp := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/nobody").
		AndRespondWithString("ok", 200)
	ms.AddExpectation(exp)

	req, _ := http.NewRequest("GET", ms.URL()+"/nobody", nil)
	resp, _ := ms.Client().Do(req)
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestMockServer_GetUnmatchedRequests ensures unmatched requests are recorded and returned correctly.
func TestMockServer_GetUnmatchedRequests(t *testing.T) {
	ms := NewMockServerWithConfig(Config{UnmatchedStatusCode: 404})
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
