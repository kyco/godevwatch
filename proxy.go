package godevwatch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)

//go:embed assets/client-reload.js
var clientReloadJS []byte

//go:embed assets/server-down.html
var serverDownHTML []byte

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ProxyServer represents the development proxy server
type ProxyServer struct {
	config       *Config
	buildTracker *BuildTracker
	wsClients    map[*websocket.Conn]bool
	watcher      *fsnotify.Watcher
}

// NewProxyServer creates a new proxy server
func NewProxyServer(config *Config) (*ProxyServer, error) {
	buildTracker := NewBuildTracker(config.BuildStatusDir)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &ProxyServer{
		config:       config,
		buildTracker: buildTracker,
		wsClients:    make(map[*websocket.Conn]bool),
		watcher:      watcher,
	}, nil
}

// Start starts the proxy server
func (ps *ProxyServer) Start() error {
	// Watch build status directory
	go ps.watchBuildStatus()

	// Poll for backend server status
	go ps.pollServerStatus()

	// Create HTTP server
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/.godevwatch-ws", ps.handleWebSocket)

	// Build status endpoint
	mux.HandleFunc("/.godevwatch-build-status", ps.handleBuildStatus)

	// Server status endpoint
	mux.HandleFunc("/.godevwatch-server-status", ps.handleServerStatus)

	// Proxy all other requests
	mux.HandleFunc("/", ps.handleProxy)

	addr := fmt.Sprintf(":%d", ps.config.ProxyPort)
	log.Printf("\033[32m✓ Proxy server running on http://localhost:%d\033[0m\n", ps.config.ProxyPort)

	return http.ListenAndServe(addr, mux)
}

// handleWebSocket handles WebSocket connections
func (ps *ProxyServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	ps.wsClients[conn] = true

	// Send initial build status
	builds, err := ps.buildTracker.GetBuilds()
	if err == nil {
		ps.sendToClient(conn, "build-status", builds)
	}

	// Keep connection alive
	go func() {
		defer func() {
			delete(ps.wsClients, conn)
			conn.Close()
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

// handleBuildStatus returns the current build status
func (ps *ProxyServer) handleBuildStatus(w http.ResponseWriter, r *http.Request) {
	builds, err := ps.buildTracker.GetBuilds()
	if err != nil {
		http.Error(w, "Failed to get build status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(builds)
}

// handleServerStatus checks if the backend server is running
func (ps *ProxyServer) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	isRunning := ps.checkBackendServer()
	w.Header().Set("Content-Type", "text/plain")
	if isRunning {
		w.Write([]byte("server-running"))
	} else {
		w.Write([]byte("server-down"))
	}
}

// handleProxy proxies requests to the backend server
func (ps *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Check if backend server is running
	if !ps.checkBackendServer() {
		w.Header().Set("Content-Type", "text/html")
		w.Write(serverDownHTML)
		return
	}

	// Create reverse proxy
	target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", ps.config.BackendPort))
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify response to inject client script
	if ps.config.InjectScript {
		proxy.ModifyResponse = func(resp *http.Response) error {
			contentType := resp.Header.Get("Content-Type")
			if strings.Contains(contentType, "text/html") {
				return ps.injectClientScript(resp)
			}
			return nil
		}
	}

	proxy.ServeHTTP(w, r)
}

// injectClientScript injects the client reload script into HTML responses
func (ps *ProxyServer) injectClientScript(resp *http.Response) error {
	// Read the response body
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	resp.Body.Close()

	// Inject script before </body>
	html := string(body)
	script := fmt.Sprintf("<script>%s</script>", clientReloadJS)
	if strings.Contains(html, "</body>") {
		html = strings.Replace(html, "</body>", script+"</body>", 1)
	}

	// Update response
	newBody := []byte(html)
	resp.Body = http.NoBody
	resp.ContentLength = int64(len(newBody))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))

	// Write the modified body
	w := resp.Request.Context().Value(http.ResponseWriterContextKey).(http.ResponseWriter)
	w.Write(newBody)

	return nil
}

// checkBackendServer checks if the backend server is running
func (ps *ProxyServer) checkBackendServer() bool {
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", ps.config.BackendPort), "-sTCP:LISTEN", "-t")
	output, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(output))) > 0
}

// watchBuildStatus watches for changes in the build status directory
func (ps *ProxyServer) watchBuildStatus() {
	if err := ps.watcher.Add(ps.config.BuildStatusDir); err != nil {
		// Directory might not exist yet
		return
	}

	for {
		select {
		case event, ok := <-ps.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Remove == fsnotify.Remove {
				ps.broadcastBuildStatus()
			}
		case err, ok := <-ps.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// pollServerStatus polls the backend server status
func (ps *ProxyServer) pollServerStatus() {
	lastStatus := false
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		isRunning := ps.checkBackendServer()
		if isRunning != lastStatus {
			lastStatus = isRunning
			status := "down"
			if isRunning {
				status = "running"
			}
			ps.broadcastToAll("server-status", map[string]string{"status": status})
		}
	}
}

// broadcastBuildStatus broadcasts build status to all connected clients
func (ps *ProxyServer) broadcastBuildStatus() {
	builds, err := ps.buildTracker.GetBuilds()
	if err != nil {
		return
	}
	ps.broadcastToAll("build-status", map[string]interface{}{"builds": builds})
}

// broadcastToAll sends a message to all connected WebSocket clients
func (ps *ProxyServer) broadcastToAll(msgType string, data interface{}) {
	for client := range ps.wsClients {
		ps.sendToClient(client, msgType, data)
	}
}

// sendToClient sends a message to a specific WebSocket client
func (ps *ProxyServer) sendToClient(client *websocket.Conn, msgType string, data interface{}) {
	message := map[string]interface{}{
		"type": msgType,
	}

	// Merge data into message
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			message[k] = val
		}
	case map[string]string:
		for k, val := range v {
			message[k] = val
		}
	default:
		message["data"] = data
	}

	if err := client.WriteJSON(message); err != nil {
		log.Printf("Failed to send message to client: %v", err)
	}
}

// Close closes the proxy server
func (ps *ProxyServer) Close() error {
	for client := range ps.wsClients {
		client.Close()
	}
	return ps.watcher.Close()
}
