package mockhttpserver

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectBasic(t *testing.T) {
	e := NewExpectation("GET", "/api/test")
	if e.Request.Method != "GET" || e.Request.Path != "/api/test" {
		t.Errorf("unexpected Expect values: %+v", e.Request)
	}
}

func TestWithQueryParams(t *testing.T) {
	e := NewExpectation("GET", "/api").WithQueryParam("id", "123").WithQueryParams(map[string]string{"type": "user"})
	if len(e.Request.QueryParams) != 2 || e.Request.QueryParams["id"] != "123" || e.Request.QueryParams["type"] != "user" {
		t.Errorf("unexpected query params: %+v", e.Request.QueryParams)
	}
}

func TestWithHeaders(t *testing.T) {
	e := NewExpectation("GET", "/api").WithHeader("X-Auth", "abc").WithHeaders(map[string]string{"X-App": "mock"})
	if len(e.Request.Headers) != 2 || e.Request.Headers["x-auth"] != "abc" || e.Request.Headers["x-app"] != "mock" {
		t.Errorf("unexpected headers: %+v", e.Request.Headers)
	}
}

func TestWithPathPattern(t *testing.T) {
	e, err := NewExpectation("GET", "").WithPathPattern(`/api/users/\d+`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("GET", "/api/users/123", nil)
	if !e.matches(r, nil) {
		t.Errorf("expected regex to match path")
	}
}

func TestWithRequestBody(t *testing.T) {
	e := NewExpectation("POST", "/api").WithRequestBody([]byte("hello"))
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("hello")) {
		t.Errorf("expected body to match")
	}
}

func TestWithRequestJSONBody(t *testing.T) {
	e, err := NewExpectation("POST", "/api").WithRequestJSONBody(`{"id":1,"name":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"id":1,"name":"test"}`)) {
		t.Errorf("expected JSON body to match")
	}
}

func TestWithPartialJSONBody(t *testing.T) {
	e, err := NewExpectation("POST", "/api").WithPartialJSONBody(`{"name":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"name":"test","age":30}`)) {
		t.Errorf("expected partial JSON body to match")
	}
}

func TestWithRequestBodyContains(t *testing.T) {
	e := NewExpectation("POST", "/api").WithRequestBodyContains("foo")
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("hello foo world")) {
		t.Errorf("expected substring body to match")
	}
}

func TestWithRequestBodyFromFileAndRespondFromFile(t *testing.T) {
	reqFile := filepath.Join("testdata", "sample-request.json")
	resFile := filepath.Join("testdata", "sample-response.json")

	if _, err := os.Stat(reqFile); err != nil {
		t.Fatalf("request file missing: %v", err)
	}
	if _, err := os.Stat(resFile); err != nil {
		t.Fatalf("response file missing: %v", err)
	}

	ms := NewMockServerWithConfig(Config{
		UnmatchedStatusCode: 404,
	})
	defer ms.Close()

	exp, err := NewExpectation("POST", "/login").WithRequestBodyFromFile(reqFile)
	if err != nil {
		t.Fatalf("failed to create expectation: %v", err)
	}
	exp, err = exp.AndRespondFromFile(resFile, 200)
	if err != nil {
		t.Fatalf("failed to set response from file: %v", err)
	}
	ms.AddExpectation(exp)

	reqBody, err := os.ReadFile(reqFile)
	if err != nil {
		t.Fatalf("failed to read request file: %v", err)
	}

	resp, err := http.Post(ms.URL()+"/login", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedResp, err := os.ReadFile(resFile)
	if err != nil {
		t.Fatalf("failed to read response file: %v", err)
	}

	if strings.TrimSpace(string(body)) != strings.TrimSpace(string(expectedResp)) {
		t.Errorf("response mismatch:\nwant: %s\ngot:  %s",
			strings.TrimSpace(string(expectedResp)),
			strings.TrimSpace(string(body)),
		)
	}
}

func TestWithCustomBodyMatcher(t *testing.T) {
	e := NewExpectation("POST", "/api").WithCustomBodyMatcher(func(b []byte) bool {
		return strings.HasPrefix(string(b), "token:")
	})
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("token:123")) {
		t.Errorf("expected custom matcher to match")
	}
}

func TestTimesAndOnce(t *testing.T) {
	e := NewExpectation("GET", "/api").Times(2)
	if e.MaxCalls == nil || *e.MaxCalls != 2 {
		t.Errorf("expected Times(2) to set MaxCalls=2")
	}
	e2 := NewExpectation("GET", "/api").Once()
	if e2.MaxCalls == nil || *e2.MaxCalls != 1 {
		t.Errorf("expected Once() to set MaxCalls=1")
	}
}

func TestWithResponseHeaders(t *testing.T) {
	e := NewExpectation("GET", "/api").WithResponseHeader("A", "1").WithResponseHeaders(map[string]string{"B": "2"})
	headers := e.Responses[len(e.Responses)-1].Headers
	if headers["A"] != "1" || headers["B"] != "2" {
		t.Errorf("unexpected response headers: %+v", headers)
	}
}

func TestMatchesFailures(t *testing.T) {
	e := NewExpectation("POST", "/api").WithHeader("X-Test", "1").WithQueryParam("id", "42").WithRequestBody([]byte("abc"))

	r, _ := http.NewRequest("GET", "/api", nil) // wrong method
	if e.matches(r, []byte("abc")) {
		t.Errorf("expected mismatch due to method")
	}

	r, _ = http.NewRequest("POST", "/wrong", nil) // wrong path
	if e.matches(r, []byte("abc")) {
		t.Errorf("expected mismatch due to path")
	}

	u, _ := url.Parse("/api?id=41") // wrong query param
	r = &http.Request{Method: "POST", URL: u, Header: http.Header{"X-Test": []string{"1"}}}
	if e.matches(r, []byte("abc")) {
		t.Errorf("expected mismatch due to query param")
	}

	u, _ = url.Parse("/api?id=42")
	r = &http.Request{Method: "POST", URL: u, Header: http.Header{"X-Test": []string{"0"}}}
	if e.matches(r, []byte("abc")) {
		t.Errorf("expected mismatch due to header")
	}

	u, _ = url.Parse("/api?id=42")
	r = &http.Request{Method: "POST", URL: u, Header: http.Header{"X-Test": []string{"1"}}}
	if e.matches(r, []byte("wrong-body")) {
		t.Errorf("expected mismatch due to body")
	}
}

func TestStringMethod(t *testing.T) {
	e := NewExpectation("GET", "/api").Once()
	s := e.String()
	if !strings.Contains(s, "GET /api") || !strings.Contains(s, "expected: 1") {
		t.Errorf("unexpected string output: %s", s)
	}
}
