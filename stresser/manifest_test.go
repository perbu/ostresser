package stresser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	// Create a temporary manifest file
	dir := t.TempDir() // Creates a temporary directory that will be cleaned up after the test
	manifestPath := filepath.Join(dir, "test_manifest.txt")

	// Test case 1: Valid manifest with mixed formatting
	testContent := `
    key1.txt
key2/file.dat
  key3/with/spaces.log  
		key4/tab/indented.bin
  
key5.zip
`
	err := os.WriteFile(manifestPath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test manifest file: %v", err)
	}

	keys, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest failed on valid file: %v", err)
	}

	// Verify the correct keys were loaded and whitespace was trimmed
	expectedKeys := []string{
		"key1.txt",
		"key2/file.dat",
		"key3/with/spaces.log",
		"key4/tab/indented.bin",
		"key5.zip",
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("Expected %d keys, got %d", len(expectedKeys), len(keys))
	}

	for i, expected := range expectedKeys {
		if i >= len(keys) {
			t.Fatalf("Missing expected key at index %d: %s", i, expected)
		}
		if keys[i] != expected {
			t.Errorf("Key at index %d incorrect. Expected: %s, Got: %s", i, expected, keys[i])
		}
	}

	// Test case 2: Non-existent file
	_, err = LoadManifest(filepath.Join(dir, "nonexistent.txt"))
	if err == nil {
		t.Error("LoadManifest should return error for non-existent file")
	}

	// Test case 3: Empty file
	emptyPath := filepath.Join(dir, "empty.txt")
	err = os.WriteFile(emptyPath, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to create empty test file: %v", err)
	}

	_, err = LoadManifest(emptyPath)
	if err == nil {
		t.Error("LoadManifest should return error for empty file")
	}

	// Test case 4: File with only whitespace
	whitespaceOnlyPath := filepath.Join(dir, "whitespace.txt")
	err = os.WriteFile(whitespaceOnlyPath, []byte("   \n  \t  \n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create whitespace test file: %v", err)
	}

	_, err = LoadManifest(whitespaceOnlyPath)
	if err == nil {
		t.Error("LoadManifest should return error for file with only whitespace")
	}
}

func TestManifestWriter(t *testing.T) {
	// Create a temporary directory for test files
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "write_test_manifest.txt")

	// Test creating a new manifest writer
	writer, err := NewManifestWriter(manifestPath)
	if err != nil {
		t.Fatalf("Failed to create manifest writer: %v", err)
	}

	// Test adding keys
	testKeys := []string{
		"test/key1.dat",
		"test/key2.dat",
		"test/subfolder/key3.dat",
		"test/key with spaces.dat",
	}

	for _, key := range testKeys {
		err := writer.AddKey(key)
		if err != nil {
			t.Errorf("Failed to add key %s: %v", key, err)
		}
	}

	// Test closing the writer
	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close manifest writer: %v", err)
	}

	// Verify the manifest file contains the keys
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest file: %v", err)
	}

	contentStr := string(content)

	for _, key := range testKeys {
		if !strings.Contains(contentStr, key+"\n") {
			t.Errorf("Manifest file missing key: %s", key)
		}
	}

	// Test loading the written manifest
	loadedKeys, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("Failed to load written manifest: %v", err)
	}

	if len(loadedKeys) != len(testKeys) {
		t.Errorf("Expected %d keys, loaded %d", len(testKeys), len(loadedKeys))
	}

	// Verify loaded keys match written keys
	for i, expected := range testKeys {
		if i >= len(loadedKeys) {
			t.Fatalf("Missing expected key at index %d: %s", i, expected)
		}
		if loadedKeys[i] != expected {
			t.Errorf("Key at index %d incorrect. Expected: %s, Got: %s", i, expected, loadedKeys[i])
		}
	}

	// Test overwriting an existing file
	writer2, err := NewManifestWriter(manifestPath)
	if err != nil {
		t.Fatalf("Failed to create manifest writer for overwrite: %v", err)
	}

	newKey := "completely_new_key.dat"
	err = writer2.AddKey(newKey)
	if err != nil {
		t.Errorf("Failed to add key while overwriting: %v", err)
	}

	err = writer2.Close()
	if err != nil {
		t.Fatalf("Failed to close manifest writer for overwrite: %v", err)
	}

	// Verify the manifest file now contains only the new key
	content, err = os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read overwritten manifest file: %v", err)
	}

	contentStr = string(content)

	// Should only contain the new key, not the original ones
	if !strings.Contains(contentStr, newKey) {
		t.Errorf("Overwritten manifest file missing new key: %s", newKey)
	}

	for _, key := range testKeys {
		if strings.Contains(contentStr, key) {
			t.Errorf("Overwritten manifest file still contains old key: %s", key)
		}
	}
}
