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
	buildTrigger   chan []string // Changed files that triggered the build
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
		buildTrigger:   make(chan []string, 1), // Non-blocking trigger with changed files
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

	// Trigger initial build with all rules
	fw.triggerBuild(nil)

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
	var changedFiles []string
	var filesMu sync.Mutex

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Check if file matches any watch pattern in build rules
			if !fw.shouldWatch(event.Name) {
				continue
			}

			// Track changed file
			filesMu.Lock()
			changedFiles = append(changedFiles, event.Name)
			filesMu.Unlock()

			// Debounce rapid file changes
			if debounceTimer != nil {
				debounceTimer.Stop()
			}

			debounceTimer = time.AfterFunc(fw.debounceTime, func() {
				filesMu.Lock()
				files := make([]string, len(changedFiles))
				copy(files, changedFiles)
				changedFiles = nil
				filesMu.Unlock()

				fw.triggerBuild(files)
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

// shouldWatch checks if a file matches any watch pattern in build rules
func (fw *FileWatcher) shouldWatch(path string) bool {
	for _, rule := range fw.config.BuildRules {
		for _, pattern := range rule.Watch {
			if fw.matchesPattern(path, pattern) {
				return true
			}
		}
	}
	return false
}

// matchesPattern checks if a file matches a watch pattern
func (fw *FileWatcher) matchesPattern(path, pattern string) bool {
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

	return false
}

// triggerBuild triggers a build, aborting current one if running
func (fw *FileWatcher) triggerBuild(changedFiles []string) {
	select {
	case fw.buildTrigger <- changedFiles:
		// Build triggered successfully
	default:
		// Build already pending, skip
	}
}

// processBuildTriggers processes build triggers (abort-and-restart pattern like watchexec)
func (fw *FileWatcher) processBuildTriggers() {
	for {
		select {
		case changedFiles := <-fw.buildTrigger:
			// If there's a current build command running, abort it
			fw.mu.Lock()
			if fw.currentBuild != nil {
				log.Println("\033[33mAborting current build due to file change...\033[0m")

				// Mark current build as aborted first
				currentBuildID, _ := fw.buildTracker.GetCurrentBuildID()
				if currentBuildID != "" {
					fw.buildTracker.SetStatus(currentBuildID, BuildStatusAborted)
				}

				fw.currentBuild.Kill()
				fw.currentBuild = nil
			}
			fw.mu.Unlock()

			// Execute new build with changed files
			fw.executeBuild(changedFiles)

		case <-fw.stopChan:
			return
		}
	}
}

// executeBuild executes the build rules based on changed files
func (fw *FileWatcher) executeBuild(changedFiles []string) {
	// Step 1: Stop the running application FIRST (before creating new build)
	// This ensures the port is freed before we try to start the new build
	if changedFiles != nil { // Only stop if this is a rebuild (not initial build)
		tempBuildID := "stopping"
		if err := fw.processManager.StopCurrentProcess(tempBuildID); err != nil {
			log.Printf("Failed to stop previous process: %v", err)
		}
	}

	// Step 2: Create new build ID and set it as current with "building" status
	buildID, err := fw.buildTracker.NewBuild()
	if err != nil {
		log.Printf("Failed to create build ID: %v", err)
		return
	}

	log.Printf("\n\033[36m[%s] Starting build...\033[0m\n", buildID)

	// Set building status
	if err := fw.buildTracker.SetStatus(buildID, BuildStatusBuilding); err != nil {
		log.Printf("Failed to set build status: %v", err)
		return
	}

	// Step 3: Determine which build rules to run
	rulesToRun := fw.determineRulesToRun(changedFiles)

	if len(rulesToRun) == 0 {
		log.Printf("\033[33m[%s] No matching build rules found\033[0m\n", buildID)
		fw.buildTracker.ClearBuild(buildID)
		return
	}

	// Step 4: Execute each matching build rule in order
	for _, rule := range rulesToRun {
		log.Printf("\033[36m[%s] Running rule: %s\033[0m\n", buildID, rule.Name)

		buildCmd := NewCommand(rule.Command)
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

		// Clear current build tracking after each rule
		fw.mu.Lock()
		fw.currentBuild = nil
		fw.mu.Unlock()
	}

	// Step 5: Build succeeded - clean up status files
	fw.buildTracker.CleanupOldFailed(buildID)
	fw.buildTracker.ClearBuild(buildID)

	log.Printf("\033[32m[%s] Build succeeded\033[0m\n", buildID)

	// Step 6: Run the application (port should be free now)
	if fw.config.RunCmd != "" {
		if err := fw.processManager.RunProcess(buildID); err != nil {
			log.Printf("Failed to start application: %v", err)
		} else {
			log.Printf("\033[32m[%s] Application started\033[0m\n", buildID)
		}
	}
}

// determineRulesToRun determines which build rules should run based on changed files
func (fw *FileWatcher) determineRulesToRun(changedFiles []string) []BuildRule {
	// If no specific files changed (initial build), run all rules
	if len(changedFiles) == 0 {
		return fw.config.BuildRules
	}

	// Track which rules need to run
	ruleMatches := make(map[int]bool)

	for _, file := range changedFiles {
		for i, rule := range fw.config.BuildRules {
			for _, pattern := range rule.Watch {
				if fw.matchesPattern(file, pattern) {
					ruleMatches[i] = true
					break
				}
			}
		}
	}

	// Build list of rules to run in order
	var rulesToRun []BuildRule
	for i, rule := range fw.config.BuildRules {
		if ruleMatches[i] {
			rulesToRun = append(rulesToRun, rule)
		}
	}

	return rulesToRun
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
