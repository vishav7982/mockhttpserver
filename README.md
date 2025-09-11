# mockhttpserver

A **lightweight and flexible HTTP mock server for Go** — built on `httptest.Server` to make client testing simple.
# mockhttpserver

![Go CI](https://github.com/vishav7982/mockhttpserver/actions/workflows/mockhttpserver-ci.yml/badge.svg?branch=main)
[![Coverage](https://codecov.io/gh/vishav7982/mockhttpserver/branch/main/graph/badge.svg)](https://codecov.io/gh/vishav7982/mockhttpserver)
---

## Features
- Flexible request matching: HTTP method, path, headers, query params, and body.
- Support for path variables and regular expressions.
- Multiple response types: string, file, or custom function.
- Sequential responses for repeated calls.
- MaxCalls enforcement and unmatched request tracking.
- Thread-safe with call count tracking.
- Middleware support and verbose logging.
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

Contributions are welcome! 🎉 See [CONTRIBUTION.md](./CONTRIBUTING.md) for more details.



