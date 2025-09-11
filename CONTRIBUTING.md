# Contributing to mockhttpserver

🎉 First off, thank you for considering contributing to **mockhttpserver**! Your help makes this project better for everyone.

This guide will walk you through the process of contributing, reporting issues, and submitting changes.

---

## Table of Contents

- [How to Contribute](#how-to-contribute)
- [Reporting Issues](#reporting-issues)
- [Submitting Changes](#submitting-changes)
- [Code Guidelines](#code-guidelines)
- [Testing](#testing)
- [Code of Conduct](#code-of-conduct)

---

## How to Contribute

You can contribute in several ways:

- Reporting bugs or issues
- Suggesting new features or enhancements
- Writing tests
- Improving documentation
- Submitting pull requests (PRs)

---

## Reporting Issues

If you find a bug or have a feature request:

1. Go to the [Issues](https://github.com/vishav7982/mockhttpserver/issues) page of the repository.
2. Click **New Issue**.
3. Provide a clear title and description.
4. Include steps to reproduce, expected behavior, and any relevant code snippets.

---

## Submitting Changes

1. **Fork the repository**

   ```bash
   git clone https://github.com/vishav7982/mockhttpserver.git
   cd mockhttpserver
   ```
2. **Create a new branch for your feature or bug fix**
   ```bash
   git checkout -b feature/my-feature
   ```
3. **Make your changes in code, tests, or documentation.**

4. **Run tests to verify everything works**
   ```bash
   go test ./...
   ```
5. **Commit your changes**
   ```bash
   git add .
   git commit -m "feat: add new feature X"
   ```
6. **Push your branch**
   ```bash
   git push origin feature/my-feature
   ```
7. **Open a Pull Request (PR) on GitHub**
- Provide a clear description of your change
- Reference related issues if applicable
- Ensure CI tests pass

# Code Guidelines
- Use builder-style chaining for expectations.
- Write thread-safe code when modifying server state.
- Keep tests isolated; avoid relying on external services.
- Use clear and descriptive names for functions and variables.
# Testing
- Ensure all tests pass:
    ```bash
    go test ./... -race -coverprofile=coverage.out -v
    ```
- Add new tests for any new functionality.
- Avoid flaky tests; make sure tests are deterministic.

# Code of Conduct

Please follow the [Contributor Covenant](https://www.contributor-covenant.org/)
code of conduct. Be respectful, courteous, and helpful when interacting with the community.

Thank you for helping make **mockhttpserver** better! 💜