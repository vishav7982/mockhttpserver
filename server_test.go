package mockhttpserver

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// loadTestFile reads a file from the testdata folder.
func loadTestFile(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test file %s: %v", filename, err)
	}
	return string(data)
}

// TestMockServer_Basic validates a simple POST request with body returns expected response.
func TestMockServer_Basic(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		Expect("POST", "/hello").
			WithRequestBody("world").
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
		Expect("GET", "/search").
			WithQueryParam("q", "golang").
			WithHeader("X-Test", "true").
			AndRespondWithString("found", 200),
	)

	req, _ := http.NewRequest("GET", ms.URL()+"/search?q=golang", nil)
	req.Header.Set("X-Test", "true")
	client := ms.Client()
	resp, err := client.Do(req)
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

	exp, err := Expect("POST", "/api").WithRequestJSONBody(`{"ping":"pong"}`)
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
	_, err := Expect("POST", "/api").WithRequestJSONBody(`{"invalid":json}`)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestMockServer_PartialJSONMatching tests partial JSON body matching.
func TestMockServer_PartialJSONMatching(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp, err := Expect("POST", "/api").WithPartialJSONBody(`{"name":"test"}`)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

	ms.AddExpectation(exp.AndRespondWithString("matched", 200))

	// This should match because it contains the required "name":"test"
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

	exp, err := Expect("GET", "").WithPathPattern("/users/\\d+")
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

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

	exp := Expect("GET", "/test").
		AndRespondWithString("ok", 200).
		Times(2)

	ms.AddExpectation(exp)

	// First call
	resp1, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp1.Body)

	if exp.CallCount() != 1 {
		t.Errorf("expected call count 1, got %d", exp.CallCount())
	}

	// Second call
	resp2, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp2.Body)

	if exp.CallCount() != 2 {
		t.Errorf("expected call count 2, got %d", exp.CallCount())
	}

	// Verify expectations are met
	if err := ms.VerifyExpectations(); err != nil {
		t.Errorf("verification failed: %v", err)
	}
}

// TestMockServer_UnmetExpectations tests verification of unmet expectations.
func TestMockServer_UnmetExpectations(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp := Expect("GET", "/test").
		AndRespondWithString("ok", 200).
		Times(3)

	ms.AddExpectation(exp)

	// Only make 1 call instead of expected 3
	resp, err := http.Get(ms.URL() + "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	safeClose(t, resp.Body)

	// Verification should fail
	if err := ms.VerifyExpectations(); err == nil {
		t.Error("expected verification to fail, got nil")
	}
}

// TestMockServer_ConcurrentRequests tests thread safety with concurrent requests.
func TestMockServer_ConcurrentRequests(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp := Expect("GET", "/concurrent").
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
	if exp.CallCount() != expectedCalls {
		t.Errorf("expected %d calls, got %d", expectedCalls, exp.CallCount())
	}
}

// TestMockServer_ResponseFromFile serves a JSON file as response body.
func TestMockServer_ResponseFromFile(t *testing.T) {
	// Create test data directory and file
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("failed to create testdata directory: %v", err)
	}

	testResponse := `{"status":"success","data":{"id":123,"name":"test"}}`
	if err := os.WriteFile("testdata/response.json", []byte(testResponse), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	defer os.RemoveAll("testdata")

	ms := NewMockServer()
	defer ms.Close()

	exp, err := Expect("GET", "/api").AndRespondFromFile("testdata/response.json", 200)
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
	if string(got) != testResponse {
		t.Errorf("response body mismatch:\nwant: %s\ngot:  %s", testResponse, string(got))
	}
}

// TestMockServer_RequestBodyFromFile uses request body from file.
func TestMockServer_RequestBodyFromFile(t *testing.T) {
	// Create test data
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("failed to create testdata directory: %v", err)
	}

	testRequest := `{"query":"test","filters":{"active":true}}`
	if err := os.WriteFile("testdata/request.json", []byte(testRequest), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	defer os.RemoveAll("testdata")

	ms := NewMockServer()
	defer ms.Close()

	exp, err := Expect("POST", "/api").WithRequestBodyFromFile("testdata/request.json")
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}

	ms.AddExpectation(exp.AndRespondWithString("matched", 200))

	resp, err := http.Post(ms.URL()+"/api", "application/json",
		bytes.NewReader([]byte(testRequest)))
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
		Expect("GET", "/headers").
			WithResponseHeader("Content-Type", "application/json").
			WithResponseHeader("X-Custom", "test-value").
			AndRespondWithString(`{"message":"hello"}`, 200),
	)

	resp, err := http.Get(ms.URL() + "/headers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}

	if custom := resp.Header.Get("X-Custom"); custom != "test-value" {
		t.Errorf("expected X-Custom 'test-value', got '%s'", custom)
	}
}

// TestMockServer_ExpectationManagement tests adding, removing, and clearing expectations.
func TestMockServer_ExpectationManagement(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	exp1 := Expect("GET", "/test1").AndRespondWithString("ok1", 200)
	exp2 := Expect("GET", "/test2").AndRespondWithString("ok2", 200)

	ms.AddExpectation(exp1)
	ms.AddExpectation(exp2)

	// Test both expectations work
	resp1, _ := http.Get(ms.URL() + "/test1")
	safeClose(t, resp1.Body)
	resp2, _ := http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp1.StatusCode != 200 || resp2.StatusCode != 200 {
		t.Error("both expectations should work initially")
	}

	// Remove first expectation
	if !ms.RemoveExpectation(exp1) {
		t.Error("should have removed exp1")
	}

	// First should now fail, second should still work
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

	// Clear all expectations
	ms.ClearExpectations()

	resp2, _ = http.Get(ms.URL() + "/test2")
	safeClose(t, resp2.Body)

	if resp2.StatusCode != 418 {
		t.Error("all expectations should be cleared")
	}
}

// TestMockServer_UnmatchedRequests tests tracking of unmatched requests.
func TestMockServer_UnmatchedRequests(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	// Make some unmatched requests
	http.Get(ms.URL() + "/unknown1")
	http.Post(ms.URL()+"/unknown2", "text/plain", strings.NewReader("test"))

	unmatched := ms.GetUnmatchedRequests()
	if len(unmatched) != 2 {
		t.Errorf("expected 2 unmatched requests, got %d", len(unmatched))
	}

	if unmatched[0].Method != "GET" || unmatched[0].URL != "/unknown1" {
		t.Error("first unmatched request details incorrect")
	}

	if unmatched[1].Method != "POST" || unmatched[1].URL != "/unknown2" {
		t.Error("second unmatched request details incorrect")
	}

	// Clear unmatched requests
	ms.ClearUnmatchedRequests()

	unmatched = ms.GetUnmatchedRequests()
	if len(unmatched) != 0 {
		t.Errorf("expected 0 unmatched requests after clearing, got %d", len(unmatched))
	}
}

// TestMockServer_RequestBodyContains tests substring matching for request bodies.
func TestMockServer_RequestBodyContains(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		Expect("POST", "/search").
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
		MaxBodySize: 10, // Very small limit
	}

	ms := NewMockServerWithConfig(config)
	defer ms.Close()

	largeBody := strings.Repeat("x", 100) // Larger than limit
	resp, err := http.Post(ms.URL()+"/test", "text/plain",
		strings.NewReader(largeBody))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	// Should return 400 for body too large
	if resp.StatusCode != 400 {
		t.Errorf("expected status 400 for large body, got %d", resp.StatusCode)
	}
}
