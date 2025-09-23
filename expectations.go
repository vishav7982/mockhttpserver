package moxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// NewExpectation creates a new default Expectation.
func NewExpectation() *Expectation {
	return &Expectation{
		Request:           RequestExpectation{},
		Responses:         []ResponseDefinition{},
		NextResponseIndex: 0,
		InvocationCount:   0,
		MaxCalls:          nil, //unlimited by default
	}
}

// WithRequestMethod sets the HTTP method for this expectation.
// Example: .WithRequestMethod("GET")
func (e *Expectation) WithRequestMethod(method string) *Expectation {
	e.Request.Method = method
	return e
}

// WithPath sets a path pattern for the Expectation.
// It converts curly-brace path variables to regex automatically.
// Example: .WithPath("/api/{id}/foo/{name}")
func (e *Expectation) WithPath(pattern string) *Expectation {
	regexPattern := convertBracesToRegex(pattern)
	compiled, err := regexp.Compile(regexPattern)
	if err != nil {
		panic(fmt.Sprintf("invalid path pattern %q: %v", pattern, err))
	}
	e.Request.PathPattern = compiled
	return e
}

// WithPathVariable adds a single expected path variable (for use with named capture groups).
// Example: .WithPathVariable("id", "123")
func (e *Expectation) WithPathVariable(key, value string) *Expectation {
	if e.Request.PathVariables == nil {
		e.Request.PathVariables = make(map[string]string)
	}
	e.Request.PathVariables[key] = value
	return e
}

// WithPathVariables adds multiple expected path variables at once.
// Example: .WithPathVariables(map[string]string{"id": "123", "name": "john"})
func (e *Expectation) WithPathVariables(vars map[string]string) *Expectation {
	if e.Request.PathVariables == nil {
		e.Request.PathVariables = make(map[string]string)
	}
	for k, v := range vars {
		e.Request.PathVariables[k] = v
	}
	return e
}

func convertBracesToRegex(pattern string) string {
	// Replace {var} with (?P<var>[^/]+)
	re := regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)}`)
	result := re.ReplaceAllString(pattern, `(?P<$1>[^/]+)`)
	return "^" + result + "$"
}

// WithQueryParam adds a query parameter matcher to the Expectation.
// Example: .WithQueryParam("id", "123")
func (e *Expectation) WithQueryParam(key, value string) *Expectation {
	if e.Request.QueryParams == nil {
		e.Request.QueryParams = make(map[string]string)
	}
	e.Request.QueryParams[key] = value
	return e
}

// WithQueryParams adds multiple query parameter matchers at once.
// Example: .WithQueryParams(map[string]string{"id": "123", "type": "user"})
func (e *Expectation) WithQueryParams(params map[string]string) *Expectation {
	if e.Request.QueryParams == nil {
		e.Request.QueryParams = make(map[string]string)
	}
	for k, v := range params {
		e.Request.QueryParams[k] = v
	}
	return e
}

// WithHeader adds a header matcher to the Expectation.
// Keys are normalized to lowercase for case-insensitive matching.
// Example: .WithHeader("Authorization", "Bearer token")
func (e *Expectation) WithHeader(key, value string) *Expectation {
	if e.Request.Headers == nil {
		e.Request.Headers = make(map[string]string)
	}
	e.Request.Headers[strings.ToLower(key)] = value
	return e
}

// WithHeaders adds multiple header matchers at once.
// Keys are normalized to lowercase for case-insensitive matching.
// Example: .WithHeaders(map[string]string{"Content-Type": "application/json", "X-API-Key": "secret"})
func (e *Expectation) WithHeaders(headers map[string]string) *Expectation {
	if e.Request.Headers == nil {
		e.Request.Headers = make(map[string]string)
	}
	for k, v := range headers {
		e.Request.Headers[strings.ToLower(k)] = v
	}
	return e
}

// WithRequestBody sets the expected raw request body for this Expectation.
// Example: .WithRequestBody("{\"name\":\"test\"}")
func (e *Expectation) WithRequestBody(body []byte) *Expectation {
	e.Request.Body = body
	e.Request.BodyMatcher = nil
	e.Request.BodyFromFile = false
	return e
}

// WithRequestBodyString sets the expected raw request body as a string.
// Example: .WithRequestBodyString("{\"name\":\"test\"}")
func (e *Expectation) WithRequestBodyString(body string) *Expectation {
	return e.WithRequestBody([]byte(body))
}

// WithRequestBodyFromFile sets the expected request body from a file.
// Supports any file type (JSON, binary, text).
func (e *Expectation) WithRequestBodyFromFile(filepath string) *Expectation {
	data, err := os.ReadFile(filepath)
	if err != nil {
		panic(fmt.Errorf("unable to read file %q: %w", filepath, err))
	}
	e.Request.Body = data
	e.Request.BodyFromFile = true
	e.Request.BodyMatcher = nil
	return e
}

// WithRequestJSONBody sets a JSON body matcher for this Expectation.
// Returns error if the expected JSON is invalid.
func (e *Expectation) WithRequestJSONBody(expected string) *Expectation {
	var expectedJSON interface{}
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		panic(fmt.Errorf("invalid expected JSON: %w", err))
	}

	e.Request.BodyMatcher = func(actual []byte) bool {
		var actualJSON interface{}
		if err := json.Unmarshal(actual, &actualJSON); err != nil {
			return false
		}
		return reflect.DeepEqual(expectedJSON, actualJSON)
	}
	e.Request.Body = nil
	return e
}

// WithRequestPartialJSONBody sets a partial JSON body matcher.
// Example: .WithRequestPartialJSONBody(`{"name":"test"}`)
func (e *Expectation) WithRequestPartialJSONBody(expected string) *Expectation {
	var expectedJSON map[string]interface{}
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		panic(fmt.Errorf("invalid expected JSON: %w", err))
	}

	e.Request.BodyMatcher = func(actual []byte) bool {
		var actualJSON map[string]interface{}
		if err := json.Unmarshal(actual, &actualJSON); err != nil {
			return false
		}
		return containsAll(actualJSON, expectedJSON)
	}
	e.Request.Body = nil
	return e
}

// WithRequestBodyContains sets a matcher that checks if the request body contains the given substring.
// Example: .WithRequestBodyContains("test")
func (e *Expectation) WithRequestBodyContains(substring string) *Expectation {
	e.Request.BodyMatcher = func(actual []byte) bool {
		return strings.Contains(string(actual), substring)
	}
	e.Request.Body = nil
	return e
}

// WithCustomBodyMatcher allows setting a custom function to match request bodies.
func (e *Expectation) WithCustomBodyMatcher(matcher func([]byte) bool) *Expectation {
	e.Request.BodyMatcher = matcher
	e.Request.Body = nil
	return e
}

// Times sets how many times this expectation should be called.
func (e *Expectation) Times(count int) *Expectation {
	e.MaxCalls = &count
	return e
}

// Once is equivalent to Times(1)
func (e *Expectation) Once() *Expectation {
	return e.Times(1)
}

// InvocationCounter returns how many times this expectation has been matched.
func (e *Expectation) InvocationCounter() int {
	return e.InvocationCount
}

// NextResponse explicitly moves to the next response in sequence.
// If no response exists, it creates a new one.
func (e *Expectation) NextResponse() *Expectation {
	e.CreateResponseIndex++
	if len(e.Responses) <= e.CreateResponseIndex {
		e.Responses = append(e.Responses, ResponseDefinition{
			Headers:    make(map[string]string),
			StatusCode: http.StatusOK,
		})
	}
	return e
}

// getCurrentResponse returns the current response to modify, creating one if needed
func (e *Expectation) getCurrentResponse() *ResponseDefinition {
	if len(e.Responses) == 0 {
		e.Responses = append(e.Responses, ResponseDefinition{
			Headers:    make(map[string]string),
			StatusCode: http.StatusOK,
		})
	}
	return &e.Responses[e.CreateResponseIndex]
}

// SimulateTimeout sets the Timeout to true for this expectation.
// Example: .SimulateTimeout()
func (e *Expectation) SimulateTimeout() *Expectation {
	e.getCurrentResponse().TimeoutSimulation = true
	return e
}

// AndRespondWith sets the response body and status code for the current response.
func (e *Expectation) AndRespondWith(body []byte, statusCode int) *Expectation {
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	resp := e.getCurrentResponse()
	resp.Body = body
	resp.StatusCode = statusCode
	return e
}

// AndRespondWithString is a convenience wrapper
func (e *Expectation) AndRespondWithString(body string, statusCode int) *Expectation {
	return e.AndRespondWith([]byte(body), statusCode)
}

// AndRespondFromFile sets the response body from a file and status code for the current response.
func (e *Expectation) AndRespondFromFile(filePath string, statusCode int) *Expectation {
	data, err := os.ReadFile(filePath)
	if err != nil {
		panic(fmt.Errorf("error reading file %q: %w", filePath, err))
	}
	resp := e.getCurrentResponse()
	resp.Body = data
	resp.StatusCode = statusCode
	return e
}

// WithResponseHeader sets a header for the current response
func (e *Expectation) WithResponseHeader(key, value string) *Expectation {
	resp := e.getCurrentResponse()
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	resp.Headers[key] = value
	return e
}

// WithResponseHeaders sets multiple headers for the current response
func (e *Expectation) WithResponseHeaders(headers map[string]string) *Expectation {
	resp := e.getCurrentResponse()
	if resp.Headers == nil {
		resp.Headers = make(map[string]string)
	}
	for k, v := range headers {
		resp.Headers[k] = v
	}
	return e
}

// WithResponseDelay sets a delay for the current response
func (e *Expectation) WithResponseDelay(d time.Duration) *Expectation {
	resp := e.getCurrentResponse()
	resp.Delay = d
	return e
}

// matches checks if a request matches this expectation.
func (e *Expectation) matches(r *http.Request, body []byte) bool {
	// --- HTTP Method Matching ---
	if r.Method != e.Request.Method {
		return false
	}

	// --- Path / PathPattern Matching ---
	if e.Request.PathPattern != nil {
		pathMatches := e.Request.PathPattern.FindStringSubmatch(r.URL.Path)
		if pathMatches == nil {
			return false
		}
		// Capture named groups from regex
		groupNames := e.Request.PathPattern.SubexpNames()
		capturedGroups := make(map[string]string, len(groupNames))
		for groupIndex, groupName := range groupNames {
			if groupIndex > 0 && groupName != "" {
				capturedGroups[groupName] = pathMatches[groupIndex]
			}
		}
		// Validate that all path variables exactly match expectation
		for variableKey, expectedValue := range e.Request.PathVariables {
			actualValue, found := capturedGroups[variableKey]
			if !found {
				// Variable not found in the request path
				return false
			}
			if expectedValue != actualValue {
				// Value mismatch → fail
				return false
			}
		}
	}
	// --- Query Parameter Matching ---
	if len(e.Request.QueryParams) > 0 {
		query := r.URL.Query()
		for paramKey, expectedValue := range e.Request.QueryParams {
			if query.Get(paramKey) != expectedValue {
				return false
			}
		}
	}
	// --- Header Matching ---
	for headerKey, expectedValue := range e.Request.Headers {
		actualHeaderValue := r.Header.Get(headerKey)
		if actualHeaderValue != expectedValue {
			return false
		}
	}
	// --- Body Matching ---
	if e.Request.BodyMatcher != nil {
		return e.Request.BodyMatcher(body)
	} else if len(e.Request.Body) > 0 && !reflect.DeepEqual(body, e.Request.Body) {
		return false
	}
	return true
}

// String returns a string representation of the expectation for debugging.
func (e *Expectation) String() string {
	path := e.Request.Path
	if e.Request.PathPattern != nil {
		path = e.Request.PathPattern.String()
	}

	expected := "any"
	if e.MaxCalls != nil {
		expected = fmt.Sprintf("%d", *e.MaxCalls)
	}

	return fmt.Sprintf("%s %s (called: %d, expected: %s)", e.Request.Method, path, e.InvocationCount, expected)
}

// containsAll checks if actualJSON contains all key-value pairs from expectedJSON
func containsAll(actual, expected map[string]interface{}) bool {
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		if !exists {
			return false
		}
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
