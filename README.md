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
- Simulate delays or timeouts.
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
### Minamal Integration Example 
Here’s a **minimal, end-to-end** example to get you started quickly:
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

**🔁 Sequential Responses**

Sometimes, you need to simulate **different responses for the same request** across multiple calls. This is useful for testing **stateful APIs**, **polling behavior**, or **retry logic**.

Use **`NextResponse()`** to define multiple responses for a single expectation. Each call to `NextResponse()` starts a **new response object** for the same expectation.
```go
e := NewExpectation().
WithRequestMethod("GET").
WithPath("/multi-seq").
WithResponseHeader("X-Step", "1").
AndRespondWithString(`{"step":"one"}`, 200).
NextResponse(). // Starts a new response; subsequent methods apply to newly created response.
WithResponseHeader("X-Step", "2").
AndRespondWithString(`{"step":"two"}`, 201).
NextResponse(). // Starts a new response; subsequent methods apply to newly created response.
WithResponseHeader("X-Step", "3").
AndRespondWithString(`{"step":"three"}`, 202)
ms.AddExpectation(e)
```
**When this is registered with the mock server**
- 1st request → returns {"step":"one"} with status 200 and header X-Step: 1
- 2nd request → returns {"step":"two"} with status 201 and header X-Step: 2
- 3rd request → returns {"step":"three"} with status 202 and header X-Step: 3
- Any further requests → repeat the last response (step three)

That’s it — define an expectation, add it to the server, and make requests against it.

**🕒 Combining with Delays and Timeouts**

You can mix **NextResponse()** with **.WithResponseDelay()** and **.SimulateTimeout()** to simulate real-world behavior — like **slow servers** or **hung requests** — and the mock server will return responses **in the exact order you defined them** sequentially. Each request will receive the next response in sequence, respecting any delay or timeout you set.

```go
e := NewExpectation().
    WithRequestMethod("GET").
    WithPath("/polling-test").
    AndRespondWithString("initial", 200).
    NextResponse().
    WithResponseDelay(100 * time.Millisecond). // simulate slow second response
    AndRespondWithString("processing", 200).
    NextResponse().
    SimulateTimeout() // simulate a hung request on 3rd call

ms.AddExpectation(e)
```
**This setup will behave as follows:**
- First request → Immediate response "initial".
- Second request → Delayed 100 ms before returning "processing".
- Third request → Simulates a server that never replies (client should hit timeout).

📖 For more extensive usage examples — including headers, query parameters, sequential responses, response delays, simulated server timeouts, custom responders, unmatched request handling etc. — see [mock_server_test.go](./mock_server_test.go).
## Why Use It ?
- Eliminate flaky network calls in tests
- Verify client code sends requests correctly
- Reproducible, isolated tests for CI/CD pipelines

# Contributing
## Contributing

Contributions are welcome! 🎉 See [CONTRIBUTION.md](./CONTRIBUTING.md) for more details.



