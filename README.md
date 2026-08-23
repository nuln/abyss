# Abyss

Abyss is a high-performance, modular personal cloud and plugin platform built with Go and BoltDB.

## Architecture

Abyss is designed with a lightweight core that handles authentication, storage orchestration, and plugin lifecycle management.

- **Backend**: Go (Golang)
- **Database**: BoltDB (Embedded, zero-config)
- **Frontend**: Vue 3 + Vite (lives in `www/`)
- **Communication**: REST API + Server-Sent Events (SSE)

## Quick Start

### Prerequisites

- [Go](https://go.dev/) (v1.22+)
- [Node.js](https://nodejs.org/) & [pnpm](https://pnpm.io/) (for frontend)
- [Make](https://www.gnu.org/software/make/)

### Build from Source

1. **Initialize Project**:
   ```bash
   make setup
   ```
   This installs Go dependencies and frontend dependencies (via pnpm).
2. **Build Binary**:
   ```bash
   make build
   ```
3. **Run**:
   ```bash
   ./abyss
   ```

Upon first run, Abyss will generate a default `config.toml` and a random JWT secret.

## Configuration

The application is configured via `config.toml`. Key options include:
- `server.addr`: Listening address (default `:8080`).
- `server.baseURL`: Path prefix for reverse proxy setups.
- `data.dir`: Root directory for database and storage (default `data`).
- `auth.jwtSecret`: Secret used for token signing (auto-generated).

Refer to `config.toml.example` for a complete list of options.

## Features

- **Embedded Database**: Powered by BoltDB, requiring no external database setup.
- **Sub-path Support**: Can be easily hosted behind a reverse proxy using the `baseURL` setting.
- **Dynamic Frontend**: Modern Vue 3 interface that hydrates its configuration from the backend.
- **Plugin Architecture**: Extend functionality via a robust plugin system.
- **Secure by Design**: Automatic JWT secret generation and strict permission models.

## Development

- `make test`: Run all backend tests.
- `make coverage`: Generate test coverage reports.
- `make lint`: Run the project linter.
- `make clean`: Remove build artifacts.

## Documentation

Additional documentation lives in the [`docs/`](docs/README.md) directory:

- [Project Context](docs/project-context.md) - architecture, tech stack and data model.
- [Auth & Permissions](docs/auth-and-permissions.md) / [BoltDB Usage](docs/boltdb-usage.md) / [Plugin Development](docs/plugin-development.md).
- [Code Review](docs/code-review.md) - internal backend audit report.

## License

[Add License Info Here]
