# godevwatch

A development server and live reload tool for Go applications. Provides automatic rebuilds, live browser reloads, and a development proxy server.

## Features

- 🔄 **Live Reload**: Automatically reloads your browser when builds complete
- 🚀 **Development Proxy**: Intelligent proxy that shows build status and handles server downtime
- 📊 **Build Tracking**: Real-time build status notifications with timestamps
- 🎯 **Zero Config**: Works out of the box with sensible defaults
- 🛠️ **Customizable**: Configure via YAML or command-line flags

## Installation

```bash
go install github.com/yourusername/godevwatch/cmd/godevwatch@latest
```

## Quick Start

### Basic Usage

Start the dev server with default settings:

```bash
godevwatch
```

This will:
- Start a proxy server on port `3000`
- Forward requests to your backend on port `8080`
- Watch `tmp/.build-counters` for build status changes
- Inject live reload scripts into HTML responses

### With Configuration File

1. Create a config file:

```bash
godevwatch --init
```

2. Edit `godevwatch.yaml` to match your project setup

3. Run:

```bash
godevwatch
```

### Command-Line Flags

```bash
godevwatch \
  --proxy-port 3000 \
  --backend-port 8080 \
  --status-dir tmp/.build-counters \
  --inject-script=true
```

## Configuration

### YAML Configuration

Create a `godevwatch.yaml` file in your project root:

```yaml
# Port for the development proxy server
proxy_port: 3000

# Port of your backend Go server
backend_port: 8080

# Directory where build status files are stored
build_status_dir: tmp/.build-counters

# File patterns to watch for changes
watch:
  - "**/*.go"
  - "**/*.templ"

# Command to build your application
build_cmd: "templ generate && go build -o ./tmp/main ."

# Command to run your application
run_cmd: "./tmp/main"

# Whether to inject the live reload script into HTML responses
inject_script: true
```

### Integration with Build Tools

godevwatch uses a filesystem-based approach to track build status. Your build tool should create status files in the configured directory.

#### Example with watchexec

**watchexec.sh**:
```bash
#!/bin/bash
watchexec \
  --exts go,templ \
  --watch . \
  --restart \
  --stop-signal SIGTERM \
  -- ./build.sh
```

**build.sh**:
```bash
#!/bin/bash
set -e

BUILD_ID=$(date +%s)-$$
COUNTER_DIR="tmp/.build-counters"

# Create status directory
mkdir -p "$COUNTER_DIR"

# Mark build as building
touch "$COUNTER_DIR/$BUILD_ID-building"

# Run your build commands
if ! templ generate; then
    rm -f "$COUNTER_DIR/$BUILD_ID-building"
    touch "$COUNTER_DIR/$BUILD_ID-failed"
    exit 1
fi

if ! go build -o ./tmp/main .; then
    rm -f "$COUNTER_DIR/$BUILD_ID-building"
    touch "$COUNTER_DIR/$BUILD_ID-failed"
    exit 1
fi

# Build succeeded, clean up status files
rm -f "$COUNTER_DIR/$BUILD_ID-"*

# Run your application
./tmp/main
```

## How It Works

1. **Proxy Server**: godevwatch runs a reverse proxy that forwards requests to your Go backend
2. **Build Status Tracking**: Watches a directory for build status files (building, failed, aborted)
3. **WebSocket Communication**: Pushes real-time updates to the browser via WebSocket
4. **Script Injection**: Injects a lightweight client script into HTML responses for live reload
5. **Smart Reloading**: Only reloads when builds complete successfully

## Build Status Protocol

godevwatch expects build status files in this format:

```
tmp/.build-counters/
  ├── 1234567890-12345-building
  ├── 1234567891-12346-failed
  └── 1234567892-12347-aborted
```

File naming: `{timestamp}-{pid}-{status}`

Valid statuses:
- `building`: Build in progress
- `failed`: Build failed
- `aborted`: Build was interrupted

When a build completes successfully, remove all status files for that build ID.

## API Endpoints

godevwatch provides these special endpoints:

- `GET /.godevwatch-ws`: WebSocket endpoint for live reload
- `GET /.godevwatch-build-status`: JSON endpoint returning current build status
- `GET /.godevwatch-server-status`: Plain text endpoint returning server status

## Development

### Project Structure

```
godevwatch/
├── assets/              # Embedded client-side files
│   ├── client-reload.js
│   └── server-down.html
├── cmd/
│   └── godevwatch/      # CLI entry point
│       └── main.go
├── build_tracker.go     # Build status tracking
├── config.go           # Configuration management
├── proxy.go            # Proxy server implementation
├── go.mod
└── README.md
```

### Building from Source

```bash
git clone https://github.com/yourusername/godevwatch
cd godevwatch
go build -o godevwatch ./cmd/godevwatch
```

## Use as a Library

You can also use godevwatch as a library in your own Go tools:

```go
package main

import (
    "log"
    "github.com/yourusername/godevwatch"
)

func main() {
    config := godevwatch.DefaultConfig()
    config.ProxyPort = 3000
    config.BackendPort = 8080

    proxy, err := godevwatch.NewProxyServer(config)
    if err != nil {
        log.Fatal(err)
    }
    defer proxy.Close()

    if err := proxy.Start(); err != nil {
        log.Fatal(err)
    }
}
```

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Credits

Inspired by live reload tools like browser-sync and webpack-dev-server, but designed specifically for Go development workflows.
