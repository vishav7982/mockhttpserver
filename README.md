# mockhttpserver

A **lightweight and flexible HTTP mock server for Go** — built on `httptest.Server` to make client testing simple.
# mockhttpserver

![Go CI](https://github.com/vishav7982/mockhttpserver/actions/workflows/ci.yml/badge.svg)
[![Coverage](https://codecov.io/gh/vishav7982/mockhttpserver/branch/main/graph/badge.svg)](https://codecov.io/gh/vishav7982/mockhttpserver)
---

## Features
- Flexible Expectations: Match requests by HTTP method, path, headers, query parameters, and body.

- Multiple Response Types: Respond with strings, files, or custom handler functions, including sequential responses.

- MaxCalls Enforcement: Limit the number of times an expectation can be matched.

- Unmatched Request Handling: Automatically record unmatched requests, retrieve them (GetUnmatchedRequests), clear them (ClearUnmatchedRequests), and define custom responders.

- Thread-Safe: Supports concurrent requests with call count tracking for each expectation.

- Logging & Verbose Mode: Inject a custom logger and enable verbose request/response logging.

- Middleware Support: Add middleware to intercept or modify requests globally.
---

## Install

```bash
go get github.com/vishav7982/mockhttpserver
```

## Quick Start
```Go
ms := mockhttpserver.NewMockServer()
defer ms.Close()

exp := mockhttpserver.NewExpectation().
        WithRequestMethod("GET").
        WithPath("/ping").
        AndRespondWithString(`{"message":"pong"}`, 200)
ms.AddExpectation(exp)

resp, _ := http.Get(ms.URL() + "/ping")
body, _ := io.ReadAll(resp.Body)

fmt.Println(string(body)) // {"message":"pong"}
```
That’s it — define an expectation, add it to the server, and make requests against it.
### Integration Example 
```go
package main

import (
   "bytes"
   "fmt"
   "io"
   "log"
   "net/http"

   "github.com/vishav7982/mockhttpserver"
)

func main() {
   ms := mockhttpserver.NewMockServer()
   defer ms.Close()

   // GET /ping
   getExp := mockhttpserver.
      NewExpectation().
      WithRequestMethod("GET").
      WithPath("/ping").
      AndRespondWithString(`{"message":"pong"}`, 200)
   ms.AddExpectation(getExp)

   resp, err := http.Get(ms.URL() + "/ping")
   if err != nil {
      log.Fatal(err)
   }
   body, err := io.ReadAll(resp.Body)
   resp.Body.Close() // close immediately
   if err != nil {
      log.Fatal(err)
   }
   fmt.Println("GET /ping:", resp.StatusCode, string(body))

   // POST /login
   postExp := mockhttpserver.
      NewExpectation().
      WithRequestMethod("POST").
      WithPath("/login").
      WithRequestBodyString(`{"username":"alice","password":"secret"}`).
      AndRespondWithString(`{"token":"abc123","expires":3600}`, 200)
   ms.AddExpectation(postExp)

   reqBody := []byte(`{"username":"alice","password":"secret"}`)
   resp, err = http.Post(ms.URL()+"/login", "application/json", bytes.NewReader(reqBody))
   if err != nil {
      log.Fatal(err)
   }
   body, err = io.ReadAll(resp.Body)
   resp.Body.Close() // close immediately
   if err != nil {
      log.Fatal(err)
   }
   fmt.Println("POST /login:", resp.StatusCode, string(body))
}


```
📖 For more extensive usage examples — including headers, query parameters, sequential responses, custom responders, and unmatched request handling — see [mock_server_test.go](./mock_server_test.go).
## Why Use It ?
- Eliminate flaky network calls in tests
- Verify client code sends requests correctly
- Reproducible, isolated tests for CI/CD pipelines

# Contributing
## Contributing

Contributions are welcome! 🎉

If you’d like to improve **mockhttpserver**, here’s how you can help:

1. **Fork** the repository on GitHub
2. **Clone** your fork locally
   ```bash
   git clone https://github.com/vishav7982/mockhttpserver.git
   cd mockhttpserver
   ```
3. Create a new branch for your feature or fix
   ```bash
   git checkout -b feature/my-feature
   ```
4. Make your changes (ensure they are well-tested)
5. Run tests to verify everything still works
   ```bash
    go test ./...
   ```
6. Commit and push your changes
    ```bash
    git commit -m "feat: add my feature"
    git push origin feature/my-feature
    ```
7. Open a Pull Request on GitHub, clearly describe your change,reference related issues if applicable

8. Ensure CI tests pass


