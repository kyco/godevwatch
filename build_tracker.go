package godevwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BuildStatus represents the status of a build
type BuildStatus string

const (
	BuildStatusBuilding BuildStatus = "building"
	BuildStatusFailed   BuildStatus = "failed"
	BuildStatusAborted  BuildStatus = "aborted"
)

// Build represents a build with its ID and status
type Build struct {
	ID        string      `json:"id"`
	Status    BuildStatus `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
}

// BuildTracker manages build status tracking
type BuildTracker struct {
	statusDir string
}

// NewBuildTracker creates a new build tracker
func NewBuildTracker(statusDir string) *BuildTracker {
	return &BuildTracker{
		statusDir: statusDir,
	}
}

// NewBuild creates a new build ID
func (bt *BuildTracker) NewBuild() (string, error) {
	if err := os.MkdirAll(bt.statusDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create status directory: %w", err)
	}

	buildID := fmt.Sprintf("%d-%d", time.Now().Unix(), os.Getpid())
	return buildID, nil
}

// SetStatus sets the status of a build
func (bt *BuildTracker) SetStatus(buildID string, status BuildStatus) error {
	if err := os.MkdirAll(bt.statusDir, 0755); err != nil {
		return fmt.Errorf("failed to create status directory: %w", err)
	}

	// Remove old status files for this build
	pattern := filepath.Join(bt.statusDir, buildID+"-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob status files: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove old status file: %w", err)
		}
	}

	// Create new status file
	statusFile := filepath.Join(bt.statusDir, fmt.Sprintf("%s-%s", buildID, status))
	f, err := os.Create(statusFile)
	if err != nil {
		return fmt.Errorf("failed to create status file: %w", err)
	}
	f.Close()

	return nil
}

// ClearBuild removes all status files for a build
func (bt *BuildTracker) ClearBuild(buildID string) error {
	pattern := filepath.Join(bt.statusDir, buildID+"-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob status files: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove status file: %w", err)
		}
	}

	return nil
}

// CleanupOldFailed removes older failed and aborted builds when a newer build succeeds
func (bt *BuildTracker) CleanupOldFailed(currentBuildID string) error {
	currentTimestamp, err := bt.extractTimestamp(currentBuildID)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(bt.statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read status directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(filename, "-failed") && !strings.HasSuffix(filename, "-aborted") {
			continue
		}

		buildID := strings.TrimSuffix(strings.TrimSuffix(filename, "-failed"), "-aborted")
		timestamp, err := bt.extractTimestamp(buildID)
		if err != nil {
			continue
		}

		if timestamp <= currentTimestamp {
			os.Remove(filepath.Join(bt.statusDir, filename))
		}
	}

	return nil
}

// GetBuilds returns all current builds with their statuses
func (bt *BuildTracker) GetBuilds() ([]Build, error) {
	entries, err := os.ReadDir(bt.statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Build{}, nil
		}
		return nil, fmt.Errorf("failed to read status directory: %w", err)
	}

	builds := make([]Build, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		parts := strings.Split(filename, "-")
		if len(parts) < 2 {
			continue
		}

		status := BuildStatus(parts[len(parts)-1])
		buildID := strings.TrimSuffix(filename, "-"+string(status))

		timestamp, err := bt.extractTimestamp(buildID)
		if err != nil {
			continue
		}

		builds = append(builds, Build{
			ID:        buildID,
			Status:    status,
			Timestamp: time.Unix(timestamp, 0),
		})
	}

	return builds, nil
}

// extractTimestamp extracts the timestamp from a build ID
func (bt *BuildTracker) extractTimestamp(buildID string) (int64, error) {
	parts := strings.Split(buildID, "-")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid build ID format")
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return timestamp, nil
}
