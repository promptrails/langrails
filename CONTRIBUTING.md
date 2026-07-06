# Contributing to LangRails

Thank you for considering contributing to LangRails! We welcome contributions of all kinds, including bug fixes, feature additions, documentation improvements, and test coverage.

## Development Setup

1. **Prerequisites:** Go 1.26.3 or later (matching `go.mod`).
2. **Clone the repository:**
   ```bash
   git clone https://github.com/promptrails/langrails.git
   cd langrails
   ```
3. **Run tests:**
   ```bash
   make test
   ```
4. **Run linters:**
   ```bash
   make lint
   ```

## Code Guidelines

- **No external dependencies.** LangRails relies only on the Go standard library. Please do not introduce third-party dependencies.
- **Follow existing patterns.** Each provider in `llm/` follows a consistent structure (`New()`, `Complete()`, `Stream()`, `Option` functional options). Match the style of existing code.
- **Document public APIs.** All exported types, functions, and methods must have Go doc comments. Every provider package must have a `doc.go` file.
- **Write tests.** Place tests alongside the source files. Use table-driven tests where appropriate.
- **Keep zero dependencies.** If you need HTTP handling, use `net/http`. If you need JSON, use `encoding/json`.

## Pull Request Process

1. Fork the repository and create a feature branch.
2. Run `make all` (fmt → vet → lint → sec → test → build) locally before pushing.
3. Open a pull request against the `main` branch.
4. Ensure the CI pipeline passes (tests, lint, vet, gosec).
5. Maintainers will review your changes and may request adjustments.

## Adding a New Provider

1. Create a new package under `llm/<name>/`.
2. Implement the `langrails.Provider` interface (`Complete` and `Stream`).
3. If the provider is OpenAI-compatible, use `llm/compat` as the base.
4. Add a `doc.go` with package documentation.
5. Register the provider in `llm/registry.go` (add a constant + case in `New`).
6. Update the provider count in `README.md` and `docs/README.md`.
7. Add tests using a mock HTTP server.

## Code of Conduct

Please note that this project is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold its terms.
