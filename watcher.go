package godevwatch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches files and triggers builds
type FileWatcher struct {
	config         *Config
	buildTracker   *BuildTracker
	processManager *ProcessManager
	watcher        *fsnotify.Watcher
	mu             sync.Mutex
	debounceTime   time.Duration
	buildTrigger   chan bool
	currentBuild   *Command
	stopChan       chan bool
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(config *Config, buildTracker *BuildTracker) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	processManager := NewProcessManager(config, buildTracker)

	return &FileWatcher{
		config:         config,
		buildTracker:   buildTracker,
		processManager: processManager,
		watcher:        watcher,
		debounceTime:   100 * time.Millisecond,
		buildTrigger:   make(chan bool, 1), // Non-blocking trigger
		stopChan:       make(chan bool),
	}, nil
}

// Start starts watching files
func (fw *FileWatcher) Start() error {
	// Add directories to watch based on patterns
	if err := fw.addWatchPaths(); err != nil {
		return err
	}

	// Start build processor
	go fw.processBuildTriggers()

	// Start file event processor
	go fw.processFileEvents()

	// Trigger initial build
	fw.triggerBuild()

	return nil
}

// addWatchPaths adds directories to watch based on config patterns
func (fw *FileWatcher) addWatchPaths() error {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Walk directory tree and add directories
	return filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		if !info.IsDir() {
			return nil
		}

		// Skip hidden directories, vendor, node_modules, etc.
		name := filepath.Base(path)
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "tmp" {
			return filepath.SkipDir
		}

		return fw.watcher.Add(path)
	})
}

// processFileEvents processes file system events
func (fw *FileWatcher) processFileEvents() {
	var debounceTimer *time.Timer

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Check if file matches watch patterns
			if !fw.shouldWatch(event.Name) {
				continue
			}

			// Debounce rapid file changes
			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			debounceTimer = time.AfterFunc(fw.debounceTime, func() {
				fw.triggerBuild()
			})

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)

		case <-fw.stopChan:
			return
		}
	}
}

// shouldWatch checks if a file matches the watch patterns
func (fw *FileWatcher) shouldWatch(path string) bool {
	for _, pattern := range fw.config.Watch {
		// Simple glob matching
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}

		// Check extension-based matching (e.g., **/*.go)
		if strings.Contains(pattern, "**/*") {
			ext := strings.TrimPrefix(pattern, "**/*")
			if strings.HasSuffix(path, ext) {
				return true
			}
		}
	}
	return false
}

// triggerBuild triggers a build, aborting current one if running
func (fw *FileWatcher) triggerBuild() {
	select {
	case fw.buildTrigger <- true:
		// Build triggered successfully
	default:
		// Build already pending, skip
	}
}

// processBuildTriggers processes build triggers (abort-and-restart pattern like watchexec)
func (fw *FileWatcher) processBuildTriggers() {
	for {
		select {
		case <-fw.buildTrigger:
			// Abort current build if running
			fw.mu.Lock()
			if fw.currentBuild != nil {
				log.Println("\033[33mAborting current build due to file change...\033[0m")
				fw.currentBuild.Kill()
				fw.currentBuild = nil
			}
			fw.mu.Unlock()

			// Execute new build
			fw.executeBuild()

		case <-fw.stopChan:
			return
		}
	}
}

// executeBuild executes the build and run commands
func (fw *FileWatcher) executeBuild() {
	buildID, err := fw.buildTracker.NewBuild()
	if err != nil {
		log.Printf("Failed to create build ID: %v", err)
		return
	}

	log.Printf("\n\033[36m[%s] Starting build...\033[0m\n", buildID)

	// Stop any running process before building
	if err := fw.processManager.StartBuild(buildID); err != nil {
		log.Printf("Failed to stop previous process: %v", err)
	}

	// Set building status
	if err := fw.buildTracker.SetStatus(buildID, BuildStatusBuilding); err != nil {
		log.Printf("Failed to set build status: %v", err)
		return
	}

	// Parse and execute build command
	buildCmd := NewCommand(fw.config.BuildCmd)
	buildCmd.OnStdout = func(line string) {
		fmt.Println(line)
	}
	buildCmd.OnStderr = func(line string) {
		fmt.Fprintln(os.Stderr, line)
	}

	// Track current build so it can be aborted
	fw.mu.Lock()
	fw.currentBuild = buildCmd
	fw.mu.Unlock()

	if err := buildCmd.Run(); err != nil {
		// Check if it was aborted vs actual failure
		fw.mu.Lock()
		wasAborted := fw.currentBuild == nil
		fw.mu.Unlock()

		if wasAborted {
			log.Printf("\033[33m[%s] Build aborted\033[0m\n", buildID)
			fw.buildTracker.SetStatus(buildID, BuildStatusAborted)
		} else {
			log.Printf("\033[31m[%s] Build failed: %v\033[0m\n", buildID, err)
			fw.buildTracker.SetStatus(buildID, BuildStatusFailed)
		}

		fw.mu.Lock()
		fw.currentBuild = nil
		fw.mu.Unlock()
		return
	}

	// Clear current build tracking
	fw.mu.Lock()
	fw.currentBuild = nil
	fw.mu.Unlock()

	// Build succeeded, clean up status files
	fw.buildTracker.CleanupOldFailed(buildID)
	fw.buildTracker.ClearBuild(buildID)

	log.Printf("\033[32m[%s] Build succeeded\033[0m\n", buildID)

	// Run the application
	if fw.config.RunCmd != "" {
		if err := fw.processManager.RunProcess(buildID); err != nil {
			log.Printf("Failed to start application: %v", err)
		} else {
			log.Printf("\033[32m[%s] Application started\033[0m\n", buildID)
		}
	}
}

// Stop stops the file watcher
func (fw *FileWatcher) Stop() error {
	close(fw.stopChan)

	// Abort any running build
	fw.mu.Lock()
	if fw.currentBuild != nil {
		fw.currentBuild.Kill()
		fw.currentBuild = nil
	}
	fw.mu.Unlock()

	// Stop process manager
	fw.processManager.Stop()

	return fw.watcher.Close()
}
