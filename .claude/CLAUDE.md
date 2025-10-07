# CLAUDE.md - Project Guide for AI Assistants

## Project Overview

**godevwatch** is a development server and live reload tool for Go applications that provides automatic rebuilds, live browser reloads, and an intelligent development proxy server.

**Key Concept**: Similar to `watchexec --restart` but integrated with a proxy server and build tracking. When files change, the current build is immediately aborted and a new one starts.

## Quick Reference

### Entry Point
- **Main CLI**: [cmd/godevwatch/main.go](cmd/godevwatch/main.go:28-142)
- **Package**: `github.com/kyco/godevwatch`
- **Go Version**: 1.24+

### Core Architecture

```
┌─────────────────────────────────────────────────┐
│  CLI (cmd/godevwatch/main.go)                   │
│  - Handles 'init' subcommand                    │
│  - Parses flags and config                      │
│  - Port cleanup on startup                      │
│  - Creates shared BuildTracker                  │
└─────────────────────────────────────────────────┘
                    │
        ┌───────────┴───────────┐
        ▼                       ▼
┌──────────────────┐    ┌──────────────────┐
│  FileWatcher     │    │  ProxyServer     │
│  (watcher.go)    │    │  (proxy.go)      │
│                  │    │                  │
│  - fsnotify      │    │  - HTTP proxy    │
│  - Pattern match │    │  - WebSocket     │
│  - Debouncing    │    │  - Script inject │
│  - Build trigger │    │  - Status check  │
└──────────────────┘    └──────────────────┘
        │                       │
        │   ┌───────────────────┘
        │   │   (both share same instance)
        ▼   ▼
┌──────────────────┐
│  BuildTracker    │
│(build_tracker.go)│
│                  │
│  - Status files  │
│  - Cleanup       │
│  - Timestamps    │
└──────────────────┘
        │
        ▼
┌──────────────────┐
│ ProcessManager   │
│ (process_mgr.go) │
│                  │
│  - Build exec    │
│  - App restart   │
│  - Process kill  │
└──────────────────┘
        │
        ▼
┌──────────────────┐
│  Command         │
│  (command.go)    │
│                  │
│  - Process group │
│  - Stream output │
│  - SIGTERM       │
└──────────────────┘
```

## File Reference Guide

### Configuration ([config.go](config.go))
```go
type Config struct {
    ProxyPort      int      // Proxy listens here (default: 3000)
    BackendPort    int      // Backend app port (default: 8080)
    BuildStatusDir string   // Status file location (default: tmp/.build-status)
    Watch          []string // Files to watch (e.g., **/*.go)
    WatchIgnore    []string // Files to ignore (e.g., **/*_templ.go)
    BuildCmd       string   // Command to build app
    RunCmd         string   // Command to run app
    InjectScript   bool     // Inject live reload script (default: true)
}
```

**Important**: File watching is auto-enabled when both `BuildCmd` and `RunCmd` are configured.

### File Watcher ([watcher.go](watcher.go))

**Pattern Matching** ([watcher.go:132-168](watcher.go:132-168)):
- `watch_ignore` patterns checked first (take precedence)
- Then `watch` patterns checked
- Supports simple glob (`*.go`) and prefix matching (`**/*.go`)

**Abort-and-Restart** ([watcher.go:180-201](watcher.go:180-201)):
- When file changes detected, current build is killed via `Command.Kill()`
- New build starts immediately
- Build status set to "aborted" in build tracker

**Key Flow**:
1. [addWatchPaths()](watcher.go:68-93) - Recursively adds directories (skips hidden, vendor, node_modules, tmp)
2. [processFileEvents()](watcher.go:96-130) - Debounces changes (100ms)
3. [processBuildTriggers()](watcher.go:180-201) - Aborts current build if running
4. [executeBuild()](watcher.go:204-277) - Executes build, then runs app on success

### Build Tracker ([build_tracker.go](build_tracker.go))

**Status Files** ([build_tracker.go:40-78](build_tracker.go:40-78)):
- Format: `{timestamp}-{pid}-{status}` (e.g., `1728300000-12345-building`)
- Stored in `BuildStatusDir` (default: `tmp/.build-status`)
- Statuses: `building`, `failed`, `aborted`
- Shared instance passed to both `FileWatcher` and `ProxyServer`

**Cleanup Strategy** ([build_tracker.go:97-134](build_tracker.go:97-134)):
- When build succeeds: removes all older failed/aborted builds
- Success = no status file (clean slate)

### Proxy Server ([proxy.go](proxy.go))

**Receives shared `BuildTracker` instance** ([proxy.go:42](proxy.go:42)) to monitor build status created by `FileWatcher`.

**Special Endpoints**:
- `/.godevwatch-ws` - WebSocket for live reload ([proxy.go:70](proxy.go:70))
- `/.godevwatch-build-status` - JSON build status ([proxy.go:73](proxy.go:73))
- `/.godevwatch-server-status` - Server up/down status ([proxy.go:76](proxy.go:76))

**Script Injection** ([proxy.go:156-164](proxy.go:156-164)):
- Intercepts HTML responses via `httputil.ReverseProxy.ModifyResponse`
- Injects [assets/client-reload.js](assets/client-reload.js) before `</body>`
- Updates Content-Length header

**Backend Detection** ([proxy.go:201-206](proxy.go:201-206)):
- Uses `lsof -i :{port} -sTCP:LISTEN -t` to check if backend is listening
- Polls every 2 seconds ([proxy.go:233-250](proxy.go:233-250))
- Shows [assets/server-down.html](assets/server-down.html) when down

**Build Status Monitoring** ([proxy.go:208-231](proxy.go:208-231)):
- Watches `BuildStatusDir` via fsnotify
- Broadcasts changes to WebSocket clients
- Uses shared `BuildTracker` instance

### Process Manager ([process_manager.go](process_manager.go))

**Lifecycle**:
1. [StartBuild()](process_manager.go:24-39) - Kills currently running app process
2. [RunProcess()](process_manager.go:42-72) - Starts new app in background
3. Streams stdout/stderr to logs

### Command ([command.go](command.go))

**Critical Implementation Details**:
- Uses `sh -c` to execute command strings ([command.go:31](command.go:31))
- Sets `Setpgid: true` to create process group ([command.go:35-37](command.go:35-37))
- [Kill()](command.go:77-95) sends SIGTERM to entire process group via `syscall.Kill(-pgid, SIGTERM)`
- Ensures child processes are also terminated

### Port Cleanup ([port_cleanup.go](port_cleanup.go))

**Cleanup Process** ([port_cleanup.go:11-64](port_cleanup.go:11-64)):
1. Find PIDs with `lsof -i :{port} -sTCP:LISTEN -t`
2. Send SIGTERM: `kill -TERM {pid}`
3. Wait 100ms
4. Check if alive: `kill -0 {pid}`
5. Force kill if needed: `kill -9 {pid}`

Called on startup for both proxy and backend ports ([cmd/godevwatch/main.go:92-98](cmd/godevwatch/main.go:92-98)).

## Common Tasks

### Adding a New Configuration Option

1. Add field to `Config` struct in [config.go](config.go:11-20)
2. Update `DefaultConfig()` in [config.go](config.go:23-34)
3. Add CLI flag in [cmd/godevwatch/main.go](cmd/godevwatch/main.go:36-43)
4. Update [godevwatch.example.yaml](godevwatch.example.yaml)
5. Update [README.md](README.md) configuration section

### Modifying Build Process

- Build execution: [watcher.go:204-277](watcher.go:204-277) - `executeBuild()`
- Process management: [process_manager.go](process_manager.go)
- Command execution: [command.go](command.go)

### Changing Proxy Behavior

- Request handling: [proxy.go:142-167](proxy.go:142-167) - `handleProxy()`
- Script injection: [proxy.go:169-199](proxy.go:169-199) - `injectClientScript()`
- WebSocket messages: [proxy.go:268-291](proxy.go:268-291) - `sendToClient()`

### Updating File Watching Logic

- Pattern matching: [watcher.go:132-168](watcher.go:132-168) - `shouldWatch()`
- Directory scanning: [watcher.go:68-93](watcher.go:68-93) - `addWatchPaths()`
- Debouncing: [watcher.go:96-130](watcher.go:96-130) - controlled by `debounceTime` (100ms)

## Build & Test

```bash
# Build
go build -o godevwatch ./cmd/godevwatch

# Run locally (requires godevwatch.yaml)
./godevwatch

# Create config
./godevwatch init

# Install
go install github.com/kyco/godevwatch/cmd/godevwatch@latest
```

## Dependencies

- **fsnotify/fsnotify v1.8.0** - File system event notifications (cross-platform)
- **gorilla/websocket v1.5.3** - WebSocket protocol implementation
- **gopkg.in/yaml.v3 v3.0.1** - YAML parsing for config files

## Important Behavioral Notes

### Process Management
- All commands run via `sh -c` to support shell features (pipes, &&, etc.)
- Process groups used to ensure child process cleanup
- SIGTERM used for graceful shutdown before SIGKILL

### File Watching
- Hidden directories (`.`), `vendor`, `node_modules`, and `tmp` are automatically excluded
- Debouncing prevents excessive rebuilds during rapid file changes
- Pattern matching checks `watch_ignore` before `watch` (ignore takes precedence)

### Build Status
- Build success = no status file (clean state)
- Status files persist for failed/aborted builds until next success
- Timestamps enable sorting and cleanup of old builds
- **Shared `BuildTracker` instance**: Both `FileWatcher` and `ProxyServer` receive the same instance to ensure they monitor the same build status

### Proxy Server
- Injects live reload script only into HTML responses (checks Content-Type)
- Backend server check uses lsof (macOS/Linux specific)
- WebSocket reconnects automatically on disconnect (2s delay)

### Client-Side Behavior
- Browser reloads when build completes (status file disappears)
- Shows notification for building/failed/aborted states
- Reconnects WebSocket on connection loss

## Platform Considerations

**macOS/Linux Only**:
- `lsof` command for port checking ([port_cleanup.go:14](port_cleanup.go:14), [proxy.go:203](proxy.go:203))
- `kill` command for process termination ([port_cleanup.go:36](port_cleanup.go:36))
- Process group management via `syscall.Setpgid` ([command.go:35-37](command.go:35-37))

**Windows Compatibility**: Would require platform-specific implementations for port checking and process management.

## Debugging Tips

1. **Build not triggering**: Check if files match `watch` patterns and not in `watch_ignore`
2. **Process not dying**: Check process group management in [command.go:77-95](command.go:77-95)
3. **Port conflicts**: Check [port_cleanup.go](port_cleanup.go) cleanup logic
4. **Live reload not working**: Verify script injection in [proxy.go:169-199](proxy.go:169-199)
5. **Build status not updating**: Check filesystem watcher in [proxy.go:208-231](proxy.go:208-231)

## Code Style Notes

- ANSI color codes used for console output (e.g., `\033[32m` for green)
- Logging uses standard `log` package
- Errors wrapped with `fmt.Errorf` and `%w` for error chains
- Goroutines used for background tasks (WebSocket watching, polling, streaming output)
- Mutexes protect shared state (current command, WebSocket clients)
