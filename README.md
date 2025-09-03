# mockhttpserver

A **lightweight and flexible HTTP mock server for Go** — built on `httptest.Server` to make client testing simple.

---

## Features

- Define expectations: method, path, headers, query, and body
- Respond with strings, files, or custom functions
- Customizable unmatched request handling
- Thread-safe with call count tracking
- Middleware support

---

## Install

```bash
go get github.com/vishav7982/mockhttpserver
```

## Quick Start
```Go
ms := mockhttpserver.NewMockServer()
defer ms.Close()

exp, _ := mockhttpserver.Expect("GET", "/ping").
    AndRespond(200, `{"message":"pong"}`)
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
    "os"
    "github.com/vishav7982/mockhttpserver"
)

func main() {
    // Start mock server
    ms := mockhttpserver.NewMockServer()
    defer ms.Close()
    // Setup expectation: POST /login with body from file
    exp, err := mockhttpserver.Expect("POST", "/login").
        WithRequestBodyFromFile("testdata/sample-request.json").
        AndRespondFromFile("testdata/sample-response.json", 200)
    if err != nil {
        log.Fatalf("failed to create expectation: %v", err)
    }
    ms.AddExpectation(exp)
    // Read request body from testdata folder
    reqBody, err := os.ReadFile("testdata/sample-request.json")
    if err != nil {
        log.Fatalf("failed to read request file: %v", err)
    }
    // Perform HTTP request against mock server
    resp, err := http.Post(ms.URL()+"/login", "application/json", bytes.NewReader(reqBody))
    if err != nil {
        log.Fatalf("request failed: %v", err)
    }
    defer resp.Body.Close()
    // Print response
    body, _ := io.ReadAll(resp.Body)
    fmt.Println("Status:", resp.StatusCode)
    fmt.Println("Response:", string(body))
}
```
📖 For more examples(headers, query params, custom responders, unmatched handlers etc.), see [server_test.go](./server_test.go).
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

