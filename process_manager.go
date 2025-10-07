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

// StopCurrentProcess stops any currently running application process
func (pm *ProcessManager) StopCurrentProcess(buildID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Kill any currently running process and WAIT for it to terminate
	if pm.currentCmd != nil {
		log.Printf("\033[33m[%s] Stopping previous process...\033[0m\n", buildID)
		if err := pm.currentCmd.Kill(); err != nil {
			// Only log if it's not already finished
			if err.Error() != "os: process already finished" {
				log.Printf("Warning: Error killing process: %v", err)
			}
		}
		pm.currentCmd = nil
		log.Printf("\033[33m[%s] Previous process stopped\033[0m\n", buildID)
	}

	return nil
}

// RunProcess runs the application process (assumes previous process already stopped)
func (pm *ProcessManager) RunProcess(buildID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

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
