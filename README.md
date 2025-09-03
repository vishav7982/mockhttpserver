# MockServer Go Library

A lightweight and flexible HTTP mock server for Go, designed to help developers easily test HTTP clients with **expectations**, **request/response matching**, and **file-based payloads**.

---

## Features

- Define **expectations** for HTTP requests (method, path, query parameters, headers, body).
- Match requests by **exact string** or **JSON body**.
- Serve responses from **string** or **file**.
- Support **request/response from files** for data-driven tests.
- Thread-safe and minimal setup.
- Optional **logger** support for debugging unexpected requests.
- Returns `418 I'm a Teapot` for unmatched requests by default.

---

## Installation

```bash
go get github.com/vishav7982/mockserver
