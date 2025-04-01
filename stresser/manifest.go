package stresser

import (
	"bufio"
	"fmt"
	"os"
	"strings" // Import the strings package
	"sync"
)

// LoadManifest reads object keys from the specified file path.
// It skips empty lines and trims whitespace from each key.
func LoadManifest(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file %s: %w", filePath, err)
	}
	defer file.Close() // Ensure file is closed

	var keys []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		// Basic trim, potentially add more validation if needed
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			keys = append(keys, trimmed)
		}
	}

	// Check for errors during scanning (e.g., read errors)
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading manifest file %s: %w", filePath, err)
	}

	// Check if any keys were actually loaded
	if len(keys) == 0 {
		return nil, fmt.Errorf("manifest file %s is empty or contains no valid keys", filePath)
	}

	return keys, nil
}

// ManifestWriter allows for concurrent writing to a manifest file
type ManifestWriter struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
}

// NewManifestWriter creates a new manifest writer
func NewManifestWriter(filePath string) (*ManifestWriter, error) {
	// Create the file with truncate if exists, create if not exists
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest file %s: %w", filePath, err)
	}

	return &ManifestWriter{
		filePath: filePath,
		file:     file,
		writer:   bufio.NewWriter(file),
	}, nil
}

// AddKey adds a key to the manifest file
func (mw *ManifestWriter) AddKey(key string) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	// Write the key with a newline
	_, err := mw.writer.WriteString(key + "\n")
	if err != nil {
		return fmt.Errorf("failed to write key to manifest: %w", err)
	}

	// Flush periodically to ensure data is written
	return mw.writer.Flush()
}

// Close closes the manifest writer and flushes any buffered data
func (mw *ManifestWriter) Close() error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	// Flush any remaining buffered data
	if err := mw.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush manifest writer: %w", err)
	}

	// Close the file
	if err := mw.file.Close(); err != nil {
		return fmt.Errorf("failed to close manifest file: %w", err)
	}

	return nil
}
