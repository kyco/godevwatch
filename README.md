# godevwatch

A development server and live reload tool for Go applications. Provides automatic rebuilds, live browser reloads, and a development proxy server.

## Features

- 🔄 **Live Reload**: Automatically reloads your browser when builds complete
- 🚀 **Development Proxy**: Intelligent proxy that shows build status and handles server downtime
- 📊 **Build Tracking**: Real-time build status notifications with timestamps
- 🎯 **Zero Config**: Works out of the box with sensible defaults
- 🛠️ **Customizable**: Configure via YAML or command-line flags
- 🧹 **Port Cleanup**: Automatically kills processes on configured ports before starting

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

### Command-Line Flags

```bash
godevwatch \
  --proxy-port 3000 \
  --backend-port 8080 \
  --status-dir tmp/.build-status \
  --inject-script=true
```

Available flags:
- `--config <path>`: Path to configuration file
- `--proxy-port <port>`: Proxy server port (default: 3000)
- `--backend-port <port>`: Backend server port (default: 8080)
- `--status-dir <path>`: Build status directory (default: tmp/.build-status)
- `--inject-script`: Inject live reload script into HTML (default: true)
- `--watch`: Explicitly enable file watching and auto-rebuild
- `--init`: Create a default configuration file
- `--version`: Show version information

**Note:** File watching is automatically enabled when `build_cmd` and `run_cmd` are configured in your config file.

## Configuration

### YAML Configuration

Create a `godevwatch.yaml` file in your project root:

```yaml
# Port for the development proxy server
proxy_port: 3000

# Port of your backend Go server
backend_port: 8080

# Directory where build status files are stored
build_status_dir: tmp/.build-status

# File patterns to watch for changes
watch:
  - "**/*.go"
  - "**/*.templ"

# File patterns to ignore (takes precedence over watch patterns)
watch_ignore:
  - "**/*_templ.go"

# Command to build your application
build_cmd: "templ generate && go build -o ./tmp/main ."

# Command to run your application
run_cmd: "./tmp/main"

# Whether to inject the live reload script into HTML responses
inject_script: true
```

**Note:** When `build_cmd` and `run_cmd` are both configured, godevwatch automatically enables file watching mode.

## Usage

### Standard Mode (Recommended)

Use godevwatch for everything - file watching, building, running, and proxying.

**godevwatch.yaml**:
```yaml
proxy_port: 3000
backend_port: 8080
build_status_dir: tmp/.build-status
watch:
  - "**/*.go"
  - "**/*.templ"
watch_ignore:
  - "**/*_templ.go"
build_cmd: "templ generate && go build -o ./tmp/main ."
run_cmd: "./tmp/main"
inject_script: true
```

**Run**:
```bash
godevwatch
```

## How It Works

### Startup

When godevwatch starts:

1. **Port Cleanup**: Uses `lsof` to find and kill (SIGTERM) any processes listening on the proxy and backend ports
2. **File Watcher Setup**: Initializes file watching if `build_cmd` and `run_cmd` are configured
3. **Proxy Server Start**: Starts the proxy server on the configured port
4. **Initial Build**: Triggers an initial build and run cycle

### File Watching

godevwatch uses `fsnotify` to watch for file changes. When files change:

1. **Directory Watching**: Watches the current directory and all subdirectories (except hidden dirs, `vendor`, `node_modules`, and `tmp`)
2. **Pattern Matching**: Files are matched against `watch` patterns and filtered by `watch_ignore` patterns
3. **Debouncing**: Rapid changes are debounced (100ms default)
4. **Abort Current Build**: If a build is running, it's immediately killed (SIGTERM) and marked as "aborted"
5. **Process Termination**: Running app is gracefully killed before starting a new build
6. **Build Execution**: Your `build_cmd` runs (stdout/stderr are streamed to console)
7. **Status Tracking**: Build status is tracked via filesystem markers (building, failed, aborted)
8. **Application Restart**: On success, your `run_cmd` is executed
9. **Live Reload**: Browser is notified via WebSocket when build status changes

This approach mimics `watchexec --restart` behavior but integrated directly into the tool.

### Build Status Protocol

Build status is tracked via files in the status directory:

```
tmp/.build-status/
  ├── 1234567890-12345-building
  ├── 1234567891-12346-failed
  └── 1234567892-12347-aborted
```

File naming: `{timestamp}-{pid}-{status}`

The tracker automatically cleans up status files from older failed/aborted builds when a newer build succeeds, and removes all status files when a build completes successfully.

Valid statuses:
- `building`: Build in progress
- `failed`: Build failed
- `aborted`: Build was interrupted by a new file change

### Proxy Server

The proxy server:
- Forwards requests to your Go backend on the configured port
- Injects live reload script into HTML responses (when `inject_script: true`)
- Serves a "waiting" page when backend server is not running
- Provides WebSocket endpoint for real-time build status and server status updates
- Uses `lsof` to check if the backend server is listening on the configured port
- Polls backend server status every 2 seconds and broadcasts changes to clients

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
    config.BuildCmd = "templ generate && go build -o ./tmp/main ."
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
| File watching | ✅ Built-in (fsnotify) | ✅ Built-in | ✅ Built-in | ✅ watchexec |
| Build abort on change | ✅ Yes | ❌ No | ❌ No | ✅ watchexec --restart |
| Process management | ✅ Built-in | ✅ Built-in | ✅ Built-in | ⚠️ Manual |
| Live reload proxy | ✅ Built-in | ✅ Built-in | ❌ No | ⚠️ Manual |
| Proxy server | ✅ Yes (with downtime handling) | ❌ No | ❌ No | ⚠️ Separate tool |
| Build status tracking | ✅ WebSocket + file markers | ❌ No | ❌ No | ⚠️ Custom |
| Ignore patterns | ✅ `watch_ignore` | ✅ `exclude_dir` | ✅ Filters | ⚠️ Manual |
| Single binary | ✅ Yes | ✅ Yes | ✅ Yes | ❌ Multiple tools |
| Auto-enable watch mode | ✅ Yes (when configured) | ⚠️ Needs config | ⚠️ Needs config | ❌ Complex setup |

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Credits

Inspired by live reload tools like browser-sync, webpack-dev-server, air, modd, and watchexec - but designed specifically for Go development workflows with build abort-and-restart behavior.
