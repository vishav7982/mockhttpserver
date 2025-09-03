package mockhttpserver

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectBasic(t *testing.T) {
	e := Expect("GET", "/api/test")
	if e.Method != "GET" || e.Path != "/api/test" {
		t.Errorf("unexpected Expect values: %+v", e)
	}
}

func TestWithQueryParams(t *testing.T) {
	e := Expect("GET", "/api").WithQueryParam("id", "123").WithQueryParams(map[string]string{"type": "user"})
	if len(e.QueryParams) != 2 || e.QueryParams["id"] != "123" || e.QueryParams["type"] != "user" {
		t.Errorf("unexpected query params: %+v", e.QueryParams)
	}
}

func TestWithHeaders(t *testing.T) {
	e := Expect("GET", "/api").WithHeader("X-Auth", "abc").WithHeaders(map[string]string{"X-App": "mock"})
	if len(e.Headers) != 2 || e.Headers["X-Auth"] != "abc" || e.Headers["X-App"] != "mock" {
		t.Errorf("unexpected headers: %+v", e.Headers)
	}
}

func TestWithPathPattern(t *testing.T) {
	e, err := Expect("GET", "").WithPathPattern(`/api/users/\d+`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("GET", "/api/users/123", nil)
	if !e.matches(r, nil) {
		t.Errorf("expected regex to match path")
	}
}

func TestWithRequestBody(t *testing.T) {
	e := Expect("POST", "/api").WithRequestBody("hello")
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("hello")) {
		t.Errorf("expected body to match")
	}
}

func TestWithRequestJSONBody(t *testing.T) {
	e, err := Expect("POST", "/api").WithRequestJSONBody(`{"id":1,"name":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"id":1,"name":"test"}`)) {
		t.Errorf("expected JSON body to match")
	}
}

func TestWithPartialJSONBody(t *testing.T) {
	e, err := Expect("POST", "/api").WithPartialJSONBody(`{"name":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"name":"test","age":30}`)) {
		t.Errorf("expected partial JSON body to match")
	}
}

func TestWithRequestBodyContains(t *testing.T) {
	e := Expect("POST", "/api").WithRequestBodyContains("foo")
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("hello foo world")) {
		t.Errorf("expected substring body to match")
	}
}

func TestWithRequestBodyFromFileAndRespondFromFile(t *testing.T) {
	tmpdir := t.TempDir()
	reqFile := filepath.Join(tmpdir, "req.json")
	resFile := filepath.Join(tmpdir, "res.json")
	os.WriteFile(reqFile, []byte(`{"k":"v"}`), 0644)
	os.WriteFile(resFile, []byte(`{"status":"ok"}`), 0644)

	e, err := Expect("POST", "/api").WithRequestBodyFromFile(reqFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte(`{"k":"v"}`)) {
		t.Errorf("expected file body to match")
	}

	e, err = e.AndRespondFromFile(resFile, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ResBody != `{"status":"ok"}` || e.ResCode != 200 {
		t.Errorf("unexpected response: %+v", e)
	}
}

func TestWithCustomBodyMatcher(t *testing.T) {
	e := Expect("POST", "/api").WithCustomBodyMatcher(func(b []byte) bool {
		return strings.HasPrefix(string(b), "token:")
	})
	r, _ := http.NewRequest("POST", "/api", nil)
	if !e.matches(r, []byte("token:123")) {
		t.Errorf("expected custom matcher to match")
	}
}

func TestTimesAndOnce(t *testing.T) {
	e := Expect("GET", "/api").Times(2)
	if e.expectedCalls == nil || *e.expectedCalls != 2 {
		t.Errorf("expected Times(2) to set expectedCalls=2")
	}
	e2 := Expect("GET", "/api").Once()
	if e2.expectedCalls == nil || *e2.expectedCalls != 1 {
		t.Errorf("expected Once() to set expectedCalls=1")
	}
}

func TestWithResponseHeaders(t *testing.T) {
	e := Expect("GET", "/api").WithResponseHeader("A", "1").WithResponseHeaders(map[string]string{"B": "2"})
	if e.responseHeaders["A"] != "1" || e.responseHeaders["B"] != "2" {
		t.Errorf("unexpected response headers: %+v", e.responseHeaders)
	}
}

func TestMatchesFailures(t *testing.T) {
	e := Expect("POST", "/api").WithHeader("X-Test", "1").WithQueryParam("id", "42").WithRequestBody("abc")

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
	if e.matches(r, []byte("wrongbody")) {
		t.Errorf("expected mismatch due to body")
	}
}

func TestStringMethod(t *testing.T) {
	e := Expect("GET", "/api").Once()
	s := e.String()
	if !strings.Contains(s, "GET /api") || !strings.Contains(s, "expected: 1") {
		t.Errorf("unexpected string output: %s", s)
	}
}
