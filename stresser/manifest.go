package stresser

import (
	"bufio"
	"fmt"
	"os"
	"strings" // Import the strings package
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
