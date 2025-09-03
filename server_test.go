package mockserver

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// TestMockServer_NoMatch ensures unmatched requests return 418.
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

// TestMockServer_ResponseFromFile serves a JSON file as response body.
func TestMockServer_ResponseFromFile(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		Expect("POST", "/api").
			WithRequestJSONBody(`{"ping":"pong"}`).
			AndRespondFromFile("testdata/response.json", 200),
	)

	resp, err := http.Post(ms.URL()+"/api", "application/json", strings.NewReader(`{"ping":"pong"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	got, _ := io.ReadAll(resp.Body)
	want := loadTestFile(t, "response.json")

	if string(got) != want {
		t.Errorf("response body mismatch:\nwant: %s\ngot:  %s", want, string(got))
	}
}

// TestMockServer_NoExpectation tests requests with no matching expectations.
func TestMockServer_NoExpectation(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	resp, err := http.Get(ms.URL() + "/unmatched")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 418 {
		t.Errorf("expected status 418, got %d", resp.StatusCode)
	}

	got, _ := io.ReadAll(resp.Body)
	expectedBody := "Unexpected request\n"
	if string(got) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(got))
	}
}

// TestMockServer_RequestAndResponseFromFile uses both request and response files.
func TestMockServer_RequestAndResponseFromFile(t *testing.T) {
	ms := NewMockServer()
	defer ms.Close()

	ms.AddExpectation(
		Expect("POST", "/api").
			WithRequestBodyFromFile("testdata/request.json").
			AndRespondFromFile("testdata/response.json", 200),
	)

	reqBody := loadTestFile(t, "request.json")
	resp, err := http.Post(ms.URL()+"/api", "application/json", bytes.NewReader([]byte(reqBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer safeClose(t, resp.Body)

	got, _ := io.ReadAll(resp.Body)
	want := []byte(loadTestFile(t, "response.json"))

	if resp.StatusCode != 200 || !bytes.Equal(got, want) {
		t.Errorf("unexpected response\nstatus=%d\nbody=%s", resp.StatusCode, string(got))
	}
}
