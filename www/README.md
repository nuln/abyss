# Abyss UI

The frontend of Abyss, built with Vue 3, Vite, and TypeScript.

## Project Setup

This project is managed as a submodule within the Abyss core repository.

### Prerequisites

- [Node.js](https://nodejs.org/) (v18+)
- [pnpm](https://pnpm.io/) (v8+)

### Installation

```bash
pnpm install
```

## Development

To start the development server with Hot Module Replacement (HMR):

```bash
pnpm dev
```

The app will be available at `http://localhost:5173`. By default, it proxies API requests to `http://localhost:8080`.

## Build

To build the production-ready assets:

```bash
pnpm build
```

The output will be generated in the `dist` directory, which is then embedded into the Go binary by the core application.

## Project Structure

- `src/app`: Global app configuration (router, stores, etc.)
- `src/domains`: Domain-driven logic and components (Auth, Files, Settings, Tasks)
- `src/shared`: Shared utilities, UI components, and constants
- `src/plugin`: Plugin system integration and SDK
- `public`: Static assets

## Features

- **Domain-Driven Design**: Organized by functional domains for scalability.
- **Dynamic Configuration**: Loads settings and `baseURL` from the backend at runtime via `window.Abyss`.
- **Plugin System**: Supports dynamic loading of UI plugins.
- **Responsive UI**: Built with modern CSS for a premium mobile and desktop experience.
