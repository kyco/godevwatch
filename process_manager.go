package godevwatch

import (
	"log"
	"sync"
)

// ProcessManager manages the build and run process lifecycle
type ProcessManager struct {
	config       *Config
	buildTracker *BuildTracker
	currentCmd   *Command
	mu           sync.Mutex
}

// NewProcessManager creates a new process manager
func NewProcessManager(config *Config, buildTracker *BuildTracker) *ProcessManager {
	return &ProcessManager{
		config:       config,
		buildTracker: buildTracker,
	}
}

// StartBuild starts a new build, killing any running process
func (pm *ProcessManager) StartBuild(buildID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Kill any currently running process
	if pm.currentCmd != nil {
		log.Printf("\033[33m[%s] Stopping previous process...\033[0m\n", buildID)
		if err := pm.currentCmd.Kill(); err != nil {
			log.Printf("Failed to kill process: %v", err)
		}
		pm.currentCmd = nil
	}

	return nil
}

// RunProcess runs the application process
func (pm *ProcessManager) RunProcess(buildID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Kill any previously running process
	if pm.currentCmd != nil {
		if err := pm.currentCmd.Kill(); err != nil {
			log.Printf("Failed to kill previous process: %v", err)
		}
	}

	// Create new command
	cmd := NewCommand(pm.config.RunCmd)
	cmd.OnStdout = func(line string) {
		log.Println(line)
	}
	cmd.OnStderr = func(line string) {
		log.Println(line)
	}

	pm.currentCmd = cmd

	// Run in background
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("\033[33m[%s] Application exited: %v\033[0m\n", buildID, err)
		}
	}()

	return nil
}

// Stop stops all managed processes
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.currentCmd != nil {
		return pm.currentCmd.Kill()
	}

	return nil
}
