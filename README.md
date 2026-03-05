# Abyss

Abyss is a powerful, modular, and plugin-driven file server and storage management system built with Go. It is designed for high performance and scalability, featuring a clean architecture that allows for easy extension through plugins.

## Features

- **Modular Architecture**: Core system decoupled from specific storage or protocol implementations.
- **Plugin System**: Extend functionality easily using blank imports.

## Getting Started

### Prerequisites

- Go 1.25.x or later

### Build

To build the standard version:

```bash
make
```

To build the project with Pro features:

```bash
make pro
```

### Run

After building, you can run the binary:

```bash
# standard
./abyss

# pro
./abyss-pro
```

## Documentation

For more detailed documentation, please refer to the internal documentation or contact the maintainers.

## Copyright

Copyright (c) 2026 Nuln. All rights reserved. (Closed Source)
