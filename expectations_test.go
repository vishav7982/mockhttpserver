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
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api/test")
	if e.Request.Method != "GET" || e.Request.PathPattern.String() != "^/api/test$" {
		t.Errorf("unexpected Expect values: %+v", e.Request)
	}
}

func TestWithQueryParams(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		WithQueryParam("id", "123").
		WithQueryParams(map[string]string{"type": "user"})

	if len(e.Request.QueryParams) != 2 || e.Request.QueryParams["id"] != "123" || e.Request.QueryParams["type"] != "user" {
		t.Errorf("unexpected query params: %+v", e.Request.QueryParams)
	}
}

func TestWithHeaders(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		WithHeader("X-Auth", "abc").
		WithHeaders(map[string]string{"X-App": "mock"})

	if len(e.Request.Headers) != 2 || e.Request.Headers["x-auth"] != "abc" || e.Request.Headers["x-app"] != "mock" {
		t.Errorf("unexpected headers: %+v", e.Request.Headers)
	}
}

func TestWithPathPattern(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath(`/api/users/\d+`) // panic if invalid

	r, _ := http.NewRequest("GET", "/api/users/123", nil)
	if !e.matches(r, nil) {
		t.Errorf("expected regex to match path")
	}
}

func TestWithRequestBody(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestBody([]byte("hello"))

	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("hello")) {
		t.Errorf("expected body to match")
	}
}

func TestWithRequestJSONBody(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestJSONBody(`{"id":1,"name":"test"}`)

	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"id":1,"name":"test"}`)) {
		t.Errorf("expected JSON body to match")
	}
}

func TestWithPartialJSONBody(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestPartialJSONBody(`{"name":"test"}`)

	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"name":"test","age":30}`)) {
		t.Errorf("expected partial JSON body to match")
	}
}

func TestWithRequestBodyContains(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestBodyContains("foo")

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

	exp := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/login").
		WithRequestBodyFromFile(reqFile)
	exp = exp.AndRespondFromFile(resFile, 200)
	ms.AddExpectation(exp)

	reqBody, _ := os.ReadFile(reqFile)
	resp, _ := http.Post(ms.URL()+"/login", "application/json", bytes.NewReader(reqBody))
	defer safeClose(t, resp.Body)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expectedResp, _ := os.ReadFile(resFile)
	if strings.TrimSpace(string(body)) != strings.TrimSpace(string(expectedResp)) {
		t.Errorf("response mismatch:\nwant: %s\ngot:  %s",
			strings.TrimSpace(string(expectedResp)),
			strings.TrimSpace(string(body)),
		)
	}
}

func TestWithCustomBodyMatcher(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithCustomBodyMatcher(func(b []byte) bool {
			return strings.HasPrefix(string(b), "token:")
		})

	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("token:123")) {
		t.Errorf("expected custom matcher to match")
	}
}

func TestTimesAndOnce(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		Times(2)
	if e.MaxCalls == nil || *e.MaxCalls != 2 {
		t.Errorf("expected Times(2) to set MaxCalls=2")
	}
	e2 := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		Once()
	if e2.MaxCalls == nil || *e2.MaxCalls != 1 {
		t.Errorf("expected Once() to set MaxCalls=1")
	}
}

func TestWithResponseHeaders(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		WithResponseHeader("A", "1").
		WithResponseHeaders(map[string]string{"B": "2"})

	headers := e.Responses[len(e.Responses)-1].Headers
	if headers["A"] != "1" || headers["B"] != "2" {
		t.Errorf("unexpected response headers: %+v", headers)
	}
}

func TestMatchesFailures(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithHeader("X-Test", "1").
		WithQueryParam("id", "42").
		WithRequestBody([]byte("abc"))

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
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		Once()

	s := e.String()
	if !strings.Contains(s, "GET") || !strings.Contains(s, "expected: 1") {
		t.Errorf("unexpected string output: %s", s)
	}
}

func TestExactPathMatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/exact/path")

	r, _ := http.NewRequest("GET", "/exact/path", nil)
	if !e.matches(r, nil) {
		t.Errorf("expected exact path to match")
	}

	r, _ = http.NewRequest("GET", "/exact/path/wrong", nil)
	if e.matches(r, nil) {
		t.Errorf("expected path mismatch for /exact/path/wrong")
	}
}

func TestPathVariableMismatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/users/{id}").
		WithPathVariable("id", "42")

	r, _ := http.NewRequest("GET", "/users/43", nil)
	if e.matches(r, nil) {
		t.Errorf("expected mismatch due to wrong path variable")
	}
}

func TestHeaderCaseInsensitiveMatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		WithHeader("X-Custom", "value")

	r, _ := http.NewRequest("GET", "/api", nil)
	r.Header.Set("x-custom", "value")
	if !e.matches(r, nil) {
		t.Errorf("expected header match to be case-insensitive")
	}
}

func TestQueryParamMismatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api").
		WithQueryParam("q", "golang")

	u, _ := url.Parse("/api?q=java")
	r := &http.Request{Method: "GET", URL: u}
	if e.matches(r, nil) {
		t.Errorf("expected query param mismatch")
	}
}

func TestJSONBodyMismatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestJSONBody(`{"id":1}`)

	r, _ := http.NewRequest("POST", "/api", nil)
	if e.matches(r, []byte(`{"id":2}`)) {
		t.Errorf("expected JSON mismatch")
	}
}

func TestPartialJSONBodyMismatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestPartialJSONBody(`{"name":"x"}`)

	r, _ := http.NewRequest("POST", "/api", nil)
	if e.matches(r, []byte(`{"age":20}`)) {
		t.Errorf("expected partial JSON mismatch")
	}
}

func TestBodyContainsMismatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithRequestBodyContains("hello")

	r, _ := http.NewRequest("POST", "/api", nil)
	if e.matches(r, []byte("world")) {
		t.Errorf("expected body contains mismatch")
	}
}

func TestCustomBodyMatcherFalse(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("POST").
		WithPath("/api").
		WithCustomBodyMatcher(func(b []byte) bool { return false })

	r, _ := http.NewRequest("POST", "/api", nil)
	if e.matches(r, []byte("any")) {
		t.Errorf("expected custom matcher to fail")
	}
}

func TestResponseFromFileInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when response file does not exist")
		} else {
			t.Logf("caught expected panic: %v", r)
		}
	}()

	e := NewExpectation()
	e.AndRespondFromFile("nonexistent-file.json", 200) // should panic
}

func TestRequestBodyFromFileInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when request file does not exist")
		} else {
			t.Logf("caught expected panic: %v", r)
		}
	}()

	e := NewExpectation()
	e.WithRequestBodyFromFile("nonexistent-file.json") // should panic
}

func TestSequentialResponses(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/api/test")

	// First response
	e.AndRespondWithString("first", 200).
		WithResponseHeader("X-Test", "one")

	// Move to second response
	e.NextResponse().
		AndRespondWithString("second", 201).
		WithResponseHeader("X-Test", "two")

	// Move to third response
	e.NextResponse().
		AndRespondWithString("third", 202).
		WithResponseHeader("X-Test", "three")

	// Verify the responses
	if len(e.Responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(e.Responses))
	}

	tests := []struct {
		idx        int
		body       string
		statusCode int
		headerVal  string
	}{
		{0, "first", 200, "one"},
		{1, "second", 201, "two"},
		{2, "third", 202, "three"},
	}

	for _, tt := range tests {
		resp := e.Responses[tt.idx]
		if string(resp.Body) != tt.body {
			t.Errorf("response %d: expected body %q, got %q", tt.idx, tt.body, string(resp.Body))
		}
		if resp.StatusCode != tt.statusCode {
			t.Errorf("response %d: expected status %d, got %d", tt.idx, tt.statusCode, resp.StatusCode)
		}
		if resp.Headers["X-Test"] != tt.headerVal {
			t.Errorf("response %d: expected header %q, got %q", tt.idx, tt.headerVal, resp.Headers["X-Test"])
		}
	}
}

// TestExpectation_ExactPathMatch verifies exact path matching when no pattern is provided.
func TestExpectation_ExactPathMatch(t *testing.T) {
	e := NewExpectation().
		WithRequestMethod("GET").
		WithPath("/exact-path") // no pattern, exact match

	tests := []struct {
		reqPath string
		want    bool
	}{
		{"/exact-path", true},   // matches exactly
		{"/exact-path/", false}, // does not match
		{"/other-path", false},  // does not match
	}

	for i, tt := range tests {
		req, _ := http.NewRequest("GET", tt.reqPath, nil)
		got := e.matches(req, nil)
		if got != tt.want {
			t.Errorf("test %d: path %q, expected %v, got %v", i+1, tt.reqPath, tt.want, got)
		}
	}
}

// TestContainsAll verifies that containsAll correctly matches nested JSON structures.
func TestContainsAll(t *testing.T) {
	tests := []struct {
		actual   map[string]interface{}
		expected map[string]interface{}
		want     bool
	}{
		// Simple flat match
		{
			actual:   map[string]interface{}{"a": 1, "b": 2},
			expected: map[string]interface{}{"a": 1},
			want:     true,
		},
		// Nested map match
		{
			actual: map[string]interface{}{
				"a": 1,
				"b": map[string]interface{}{
					"x": 10,
					"y": 20,
				},
			},
			expected: map[string]interface{}{
				"b": map[string]interface{}{
					"x": 10,
				},
			},
			want: true,
		},
		// Nested map mismatch
		{
			actual: map[string]interface{}{
				"a": 1,
				"b": map[string]interface{}{
					"x": 10,
					"y": 20,
				},
			},
			expected: map[string]interface{}{
				"b": map[string]interface{}{
					"x": 99, // mismatch
				},
			},
			want: false,
		},
		// Key missing in actual
		{
			actual:   map[string]interface{}{"a": 1},
			expected: map[string]interface{}{"b": 2},
			want:     false,
		},
	}

	for i, tt := range tests {
		got := containsAll(tt.actual, tt.expected)
		if got != tt.want {
			t.Errorf("test %d: expected %v, got %v", i+1, tt.want, got)
		}
	}
}
