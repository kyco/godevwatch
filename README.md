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
go install github.com/kyco/godevwatch/cmd/godevwatch@latest
```

## Quick Start

### Standalone Mode (File Watching + Build + Run + Proxy)

Create a config file and run everything with one command:

```bash
# Create config
godevwatch --init

# Edit godevwatch.yaml to set your build_cmd and run_cmd

# Start everything
godevwatch
```

This will:
- Watch your files for changes (based on `watch` patterns)
- Automatically rebuild when files change
- Run your application and restart on successful builds
- Start a proxy server on port `3000`
- Provide live reload in the browser

### Proxy-Only Mode

If you want to use your own file watcher (watchexec, air, etc.), you can run just the proxy:

```bash
godevwatch --proxy-only
```

### Command-Line Flags

```bash
godevwatch \
  --proxy-port 3000 \
  --backend-port 8080 \
  --status-dir tmp/.build-counters \
  --watch \
  --inject-script=true
```

Available flags:
- `--config <path>`: Path to configuration file
- `--proxy-port <port>`: Proxy server port (default: 3000)
- `--backend-port <port>`: Backend server port (default: 8080)
- `--status-dir <path>`: Build status directory (default: tmp/.build-counters)
- `--inject-script`: Inject live reload script into HTML (default: true)
- `--watch`: Enable file watching and auto-rebuild
- `--proxy-only`: Run only the proxy server (no file watching)
- `--init`: Create a default configuration file
- `--version`: Show version information

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

When `build_cmd` and `run_cmd` are configured, godevwatch automatically enables file watching mode.

## Usage Modes

### Mode 1: All-in-One (Recommended)

Use godevwatch for everything - file watching, building, running, and proxying.

**godevwatch.yaml**:
```yaml
proxy_port: 3000
backend_port: 8080
build_status_dir: tmp/.build-counters
watch:
  - "**/*.go"
  - "**/*.templ"
build_cmd: "templ generate && go build -o ./tmp/main ."
run_cmd: "./tmp/main"
inject_script: true
```

**Run**:
```bash
godevwatch
```

### Mode 2: Proxy-Only (Legacy Integration)

Use godevwatch only for the proxy/live-reload, and use your own file watcher.

**godevwatch.yaml**:
```yaml
proxy_port: 3000
backend_port: 8080
build_status_dir: tmp/.build-counters
inject_script: true
```

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
mkdir -p "$COUNTER_DIR"

# Mark as building
touch "$COUNTER_DIR/$BUILD_ID-building"

# Build
if ! templ generate || ! go build -o ./tmp/main .; then
    rm -f "$COUNTER_DIR/$BUILD_ID-building"
    touch "$COUNTER_DIR/$BUILD_ID-failed"
    exit 1
fi

# Clean up on success
rm -f "$COUNTER_DIR/$BUILD_ID-"*

# Run
./tmp/main
```

**Run**:
```bash
# Terminal 1
./watchexec.sh

# Terminal 2
godevwatch --proxy-only
```

## How It Works

### File Watching & Build Restart

godevwatch uses `fsnotify` to watch for file changes. When files change:

1. **Debouncing**: Rapid changes are debounced (100ms default)
2. **Abort Current Build**: If a build is running, it's immediately killed (SIGTERM)
3. **Start New Build**: Fresh build starts with latest changes
4. **Process Termination**: Running app is gracefully killed before new builds
5. **Build Execution**: Your `build_cmd` runs
6. **Status Tracking**: Build status is tracked via filesystem markers (building, failed, aborted)
7. **Application Restart**: On success, your `run_cmd` is executed
8. **Live Reload**: Browser is notified via WebSocket

This approach mimics `watchexec --restart` behavior but integrated directly into the tool.

### Build Status Protocol

Build status is tracked via files in the status directory:

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

When a build completes successfully, all status files for that build are removed.

### Proxy Server

The proxy server:
- Forwards requests to your Go backend
- Injects live reload script into HTML responses
- Serves a "waiting" page when backend is down
- Provides WebSocket endpoint for real-time updates

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
├── command.go           # Command execution with process management
├── config.go            # Configuration management
├── process_manager.go   # Process lifecycle management
├── proxy.go             # Proxy server implementation
├── watcher.go           # File watching and build orchestration
├── go.mod
└── README.md
```

### Building from Source

```bash
git clone https://github.com/kyco/godevwatch
cd godevwatch
go build -o godevwatch ./cmd/godevwatch
```

## Use as a Library

You can also use godevwatch as a library in your own Go tools:

```go
package main

import (
    "log"
    "github.com/kyco/godevwatch"
)

func main() {
    config := godevwatch.DefaultConfig()
    config.ProxyPort = 3000
    config.BackendPort = 8080
    config.BuildCmd = "go build -o ./tmp/main ."
    config.RunCmd = "./tmp/main"

    buildTracker := godevwatch.NewBuildTracker(config.BuildStatusDir)

    // Start file watcher
    watcher, err := godevwatch.NewFileWatcher(config, buildTracker)
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Stop()

    if err := watcher.Start(); err != nil {
        log.Fatal(err)
    }

    // Start proxy server
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

## Comparison with Other Tools

| Feature | godevwatch | air | modd | watchexec + custom |
|---------|-----------|-----|------|--------------------|
| File watching | ✅ Built-in | ✅ Built-in | ✅ Built-in | ✅ watchexec |
| Build abort on change | ✅ Yes (--restart) | ❌ No | ❌ No | ✅ watchexec --restart |
| Process management | ✅ Built-in | ✅ Built-in | ✅ Built-in | ⚠️ Manual |
| Live reload | ✅ Built-in | ✅ Built-in | ❌ No | ⚠️ Manual |
| Proxy server | ✅ Built-in | ❌ No | ❌ No | ⚠️ Separate tool |
| Build status UI | ✅ Yes | ❌ No | ❌ No | ⚠️ Custom |
| Single binary | ✅ Yes | ✅ Yes | ✅ Yes | ❌ Multiple tools |
| Zero config | ✅ Yes | ⚠️ Needs config | ⚠️ Needs config | ❌ Complex setup |

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Credits

Inspired by live reload tools like browser-sync, webpack-dev-server, air, and watchexec - but designed specifically for Go development workflows with build queuing support.
