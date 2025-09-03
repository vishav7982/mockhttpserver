package mockserver

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
)

// Expectation defines a mock expectation for HTTP requests.
// Supports method, path, query parameters, headers, and request body.
// Also defines the mock response (status code + body).
type Expectation struct {
	Method      string
	Path        string
	ReqBody     string
	ResCode     int
	ResBody     string
	bodyMatcher func([]byte) bool
	QueryParams map[string]string
	Headers     map[string]string
}

// Expect creates a new Expectation for a given HTTP method and path.
// Example: mockserver.Expect("GET", "/api/resource")
func Expect(method, path string) *Expectation {
	return &Expectation{
		Method:  method,
		Path:    path,
		Headers: make(map[string]string),
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

// WithHeader adds a header matcher to the Expectation.
// Example: .WithHeader("Authorization", "Bearer token")
func (e *Expectation) WithHeader(key, value string) *Expectation {
	if e.Headers == nil {
		e.Headers = make(map[string]string)
	}
	e.Headers[key] = value
	return e
}

// WithRequestBody sets the expected raw request body for this Expectation.
// Example: .WithRequestBody("{\"name\":\"test\"}")
func (e *Expectation) WithRequestBody(body string) *Expectation {
	e.ReqBody = body
	return e
}

// WithRequestJSONBody sets a JSON body matcher for this Expectation.
// Example: .WithRequestJSONBody(`{"id":123,"name":"test"}`)
func (e *Expectation) WithRequestJSONBody(expected string) *Expectation {
	var expectedJSON interface{}
	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		panic(fmt.Sprintf("Invalid expected JSON: %v", err))
	}

	e.bodyMatcher = func(actual []byte) bool {
		var actualJSON interface{}
		if err := json.Unmarshal(actual, &actualJSON); err != nil {
			return false
		}
		return reflect.DeepEqual(expectedJSON, actualJSON)
	}
	return e
}

// WithRequestBodyFromFile loads the expected request body from a file.
// Example: .WithRequestBodyFromFile("testdata/request.json")
func (e *Expectation) WithRequestBodyFromFile(filepath string) *Expectation {
	data, err := os.ReadFile(filepath)
	if err != nil {
		panic("unable to read test data file: " + err.Error())
	}
	return e.WithRequestBody(string(data))
}

// AndRespondWithString sets the mock response body and status code.
// Example: .AndRespondWithString("{\"status\":\"ok\"}", 200)
func (e *Expectation) AndRespondWithString(responseBody string, statusCode int) *Expectation {
	e.ResCode = statusCode
	e.ResBody = responseBody
	return e
}

// AndRespondFromFile sets the response body from a file and status code.
// Example: .AndRespondFromFile("testdata/response.json", 200)
func (e *Expectation) AndRespondFromFile(filePath string, statusCode int) *Expectation {
	data, err := os.ReadFile(filePath)
	if err != nil {
		panic(fmt.Sprintf("error reading file %s: %v", filePath, err))
	}
	e.ResBody = string(data)
	e.ResCode = statusCode
	return e
}
