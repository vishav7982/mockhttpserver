package mockhttpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
)

// Expectation defines a mock expectation for HTTP requests.
// Supports method, path, query parameters, headers, and request body.
// Also defines the mock response (status code + body).
type Expectation struct {
	Method          string
	Path            string
	pathPattern     *regexp.Regexp
	ReqBody         string
	ResCode         int
	ResBody         string
	bodyMatcher     func([]byte) bool
	QueryParams     map[string]string
	Headers         map[string]string
	responseHeaders map[string]string
	callCount       int
	expectedCalls   *int // nil means any number of calls
}

// Expect creates a new Expectation for a given HTTP method and path.
// Example: mockserver.Expect("GET", "/api/resource")
func Expect(method, path string) *Expectation {
	return &Expectation{
		Method:          method,
		Path:            path,
		Headers:         make(map[string]string),
		responseHeaders: make(map[string]string),
	}
}

// WithQueryParam adds a query parameter matcher to the Expectation.
// Example: .WithQueryParam("id", "123")
func (e *Expectation) WithQueryParam(key, value string) *Expectation {
	if e.QueryParams == nil {
		e.QueryParams = make(map[string]string)
	}
	e.QueryParams[key] = value
	return e
}

// WithQueryParams adds multiple query parameter matchers at once.
// Example: .WithQueryParams(map[string]string{"id": "123", "type": "user"})
func (e *Expectation) WithQueryParams(params map[string]string) *Expectation {
	if e.QueryParams == nil {
		e.QueryParams = make(map[string]string)
	}
	for k, v := range params {
		e.QueryParams[k] = v
	}
	return e
}

// WithHeader adds a header matcher to the Expectation.
// Example: .WithHeader("Authorization", "Bearer token")
func (e *Expectation) WithHeader(key, value string) *Expectation {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
	return e
}

// WithHeaders adds multiple header matchers at once.
// Example: .WithHeaders(map[string]string{"Content-Type": "application/json", "X-API-Key": "secret"})
func (e *Expectation) WithHeaders(headers map[string]string) *Expectation {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	for k, v := range headers {
		e.Headers[k] = v
	}
	return e
}

// WithPathPattern sets a regex pattern for matching the path instead of exact match.
// Example: .WithPathPattern("/api/users/\\d+")
func (e *Expectation) WithPathPattern(pattern string) (*Expectation, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid path pattern %q: %w", pattern, err)
	}
	e.pathPattern = compiled
	return e, nil
}

// WithRequestBody sets the expected raw request body for this Expectation.
// Example: .WithRequestBody("{\"name\":\"test\"}")
func (e *Expectation) WithRequestBody(body string) *Expectation {
	e.ReqBody = body
	e.bodyMatcher = nil // Clear any existing body matcher
	return e
}

// WithRequestJSONBody sets a JSON body matcher for this Expectation.
// Returns error if the expected JSON is invalid.
// Example: .WithRequestJSONBody(`{"id":123,"name":"test"}`)
func (e *Expectation) WithRequestJSONBody(expected string) (*Expectation, error) {
	var expectedJSON interface{}
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		return nil, fmt.Errorf("invalid expected JSON: %w", err)
	}

	e.bodyMatcher = func(actual []byte) bool {
		var actualJSON interface{}
		if err := json.Unmarshal(actual, &actualJSON); err != nil {
			return false
		}
		return reflect.DeepEqual(expectedJSON, actualJSON)
	}
	e.ReqBody = "" // Clear raw body since we're using matcher
	return e, nil
}

// WithPartialJSONBody sets a partial JSON body matcher that checks if the actual request
// contains all the fields from the expected JSON (but can have additional fields).
// Example: .WithPartialJSONBody(`{"name":"test"}`) matches `{"name":"test","age":30}`
func (e *Expectation) WithPartialJSONBody(expected string) (*Expectation, error) {
	var expectedJSON map[string]interface{}
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		return nil, fmt.Errorf("invalid expected JSON: %w", err)
	}

	e.bodyMatcher = func(actual []byte) bool {
		var actualJSON map[string]interface{}
		if err := json.Unmarshal(actual, &actualJSON); err != nil {
			return false
		}
		return containsAll(actualJSON, expectedJSON)
	}
	e.ReqBody = "" // Clear raw body since we're using matcher
	return e, nil
}

// WithRequestBodyContains sets a matcher that checks if the request body contains the given substring.
// Example: .WithRequestBodyContains("test")
func (e *Expectation) WithRequestBodyContains(substring string) *Expectation {
	e.bodyMatcher = func(actual []byte) bool {
		return strings.Contains(string(actual), substring)
	}
	e.ReqBody = "" // Clear raw body since we're using matcher
	return e
}

// WithRequestBodyFromFile loads the expected request body from a file.
// Returns error if file cannot be read.
// Example: .WithRequestBodyFromFile("testdata/request.json")
func (e *Expectation) WithRequestBodyFromFile(filepath string) (*Expectation, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %q: %w", filepath, err)
	}
	return e.WithRequestBody(string(data)), nil
}

// WithCustomBodyMatcher allows setting a custom function to match request bodies.
// Example: .WithCustomBodyMatcher(func(body []byte) bool { return len(body) > 100 })
func (e *Expectation) WithCustomBodyMatcher(matcher func([]byte) bool) *Expectation {
	e.bodyMatcher = matcher
	e.ReqBody = "" // Clear raw body since we're using matcher
	return e
}

// Times sets how many times this expectation should be called.
// Use for verification with VerifyExpectations().
// Example: .Times(1) - expect exactly one call
func (e *Expectation) Times(count int) *Expectation {
	e.expectedCalls = &count
	return e
}

// Once is equivalent to Times(1).
func (e *Expectation) Once() *Expectation {
	return e.Times(1)
}

// CallCount returns how many times this expectation has been matched.
func (e *Expectation) CallCount() int {
	return e.callCount
}

// AndRespondWithString sets the mock response body and status code.
// Example: .AndRespondWithString("{\"status\":\"ok\"}", 200)
func (e *Expectation) AndRespondWithString(responseBody string, statusCode int) *Expectation {
	e.ResCode = statusCode
	e.ResBody = responseBody
	return e
}

// AndRespondFromFile sets the response body from a file and status code.
// Returns error if file cannot be read.
// Example: .AndRespondFromFile("testdata/response.json", 200)
func (e *Expectation) AndRespondFromFile(filePath string, statusCode int) (*Expectation, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", filePath, err)
	}
	e.ResBody = string(data)
	e.ResCode = statusCode
	return e, nil
}

// WithResponseHeader sets a response header.
// Example: .WithResponseHeader("Content-Type", "application/json")
func (e *Expectation) WithResponseHeader(key, value string) *Expectation {
	if e.responseHeaders == nil {
		e.responseHeaders = make(map[string]string)
	}
	e.responseHeaders[key] = value
	return e
}

// WithResponseHeaders sets multiple response headers at once.
func (e *Expectation) WithResponseHeaders(headers map[string]string) *Expectation {
	if e.responseHeaders == nil {
		e.responseHeaders = make(map[string]string)
	}
	for k, v := range headers {
		e.responseHeaders[k] = v
	}
	return e
}

// matches checks if a request matches this expectation
func (e *Expectation) matches(r *http.Request, body []byte) bool {
	// Match HTTP method
	if r.Method != e.Method {
		return false
	}

	// Match path (exact or pattern)
	if e.pathPattern != nil {
		if !e.pathPattern.MatchString(r.URL.Path) {
			return false
		}
	} else if r.URL.Path != e.Path {
		return false
	}

	// Match query parameters
	if len(e.QueryParams) > 0 {
		q := r.URL.Query()
		for key, expectedValue := range e.QueryParams {
			if q.Get(key) != expectedValue {
				return false
			}
		}
	}

	// Match headers
	if len(e.Headers) > 0 {
		for key, expectedValue := range e.Headers {
			if r.Header.Get(key) != expectedValue {
				return false
			}
		}
	}

	// Match body if a matcher is defined
	if e.bodyMatcher != nil {
		return e.bodyMatcher(body)
	} else if e.ReqBody != "" && e.ReqBody != string(body) {
		return false
	}

	return true
}

// String returns a string representation of the expectation for debugging
func (e *Expectation) String() string {
	path := e.Path
	if e.pathPattern != nil {
		path = e.pathPattern.String()
	}

	expected := "any"
	if e.expectedCalls != nil {
		expected = fmt.Sprintf("%d", *e.expectedCalls)
	}

	return fmt.Sprintf("%s %s (called: %d, expected: %s)", e.Method, path, e.callCount, expected)
}

// containsAll checks if actualJSON contains all key-value pairs from expectedJSON
func containsAll(actual, expected map[string]interface{}) bool {
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		if !exists {
			return false
		}

		// Recursive check for nested objects
		if expectedMap, ok := expectedValue.(map[string]interface{}); ok {
			if actualMap, ok := actualValue.(map[string]interface{}); ok {
				if !containsAll(actualMap, expectedMap) {
					return false
				}
			} else {
				return false
			}
		} else if !reflect.DeepEqual(actualValue, expectedValue) {
			return false
		}
	}
	return true
}
