package mockhttpserver

import "regexp"

// ResponseDefinition defines a mock response for an expectation.
type ResponseDefinition struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// RequestExpectation defines the expected request structure.
type RequestExpectation struct {
	Method       string
	Path         string
	PathPattern  *regexp.Regexp
	Body         []byte
	BodyMatcher  func([]byte) bool
	QueryParams  map[string]string
	Headers      map[string]string // stored as lowercase keys for case-insensitive matching
	BodyFromFile bool
}

// Expectation defines a mock expectation for HTTP requests.
// It contains the expected request and one or more sequential responses.
type Expectation struct {
	Request           RequestExpectation
	Responses         []ResponseDefinition
	InvocationCount   int
	MaxCalls          *int // nil means unlimited
	NextResponseIndex int  // tracks which response to return next
}
