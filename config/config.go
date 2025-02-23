package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppName = "open_whisper"
	RecordingsDir = "recordings"
	TranscriptionsDir = "transcriptions"
)

// GetAppDataDir returns the application data directory path and ensures all required subdirectories exist
func GetAppDataDir() (string, error) {
	// Get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create app data directory path
	appDataDir := filepath.Join(homeDir, "."+AppName)

	// Create directories if they don't exist
	for _, dir := range []string{
		appDataDir,
		filepath.Join(appDataDir, RecordingsDir),
		filepath.Join(appDataDir, TranscriptionsDir),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return appDataDir, nil
} 