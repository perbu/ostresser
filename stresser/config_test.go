package stresser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid Read Configuration",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "read",
				PutObjectSizeKB: 256,
			},
			expectError: false,
		},
		{
			name: "Valid Write Configuration",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "1m",
				Concurrency:     10,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "write",
				PutObjectSizeKB: 1024,
			},
			expectError: false,
		},
		{
			name: "Valid Mixed Configuration",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "5m",
				Concurrency:     20,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "mixed",
				PutObjectSizeKB: 512,
			},
			expectError: false,
		},
		{
			name: "Missing Duration",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "read",
				PutObjectSizeKB: 256,
			},
			expectError: true,
		},
		{
			name: "Invalid Concurrency",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     0,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "read",
				PutObjectSizeKB: 256,
			},
			expectError: true,
		},
		{
			name: "Missing ManifestPath",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "",
				OutputFile:      "results.csv",
				OperationType:   "read",
				PutObjectSizeKB: 256,
			},
			expectError: true,
		},
		{
			name: "Missing OutputFile",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "",
				OperationType:   "read",
				PutObjectSizeKB: 256,
			},
			expectError: true,
		},
		{
			name: "Invalid OperationType",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "invalid",
				PutObjectSizeKB: 256,
			},
			expectError: true,
		},
		{
			name: "Invalid PutObjectSizeKB for Write Mode",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "write",
				PutObjectSizeKB: 0,
			},
			expectError: true,
		},
		{
			name: "Invalid PutObjectSizeKB for Mixed Mode",
			config: Config{
				Endpoint:        "https://test-endpoint.com",
				Region:          "us-east-1",
				Bucket:          "test-bucket",
				Duration:        "30s",
				Concurrency:     5,
				ManifestPath:    "manifest.txt",
				OutputFile:      "results.csv",
				OperationType:   "mixed",
				PutObjectSizeKB: -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.expectError {
				t.Errorf("Validate() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary YAML file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "test_config.yaml")

	// Test case 1: Valid config file
	validConfig := `
endpoint: "https://test-endpoint.com"
region: "us-east-1"
bucket: "test-bucket"
accessKey: "test-access-key"
secretKey: "test-secret-key"
operationType: "mixed"
putObjectSizeKB: 2048
insecureSkipVerify: true
`
	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Save environment variables
	origEndpoint := os.Getenv("AWS_ENDPOINT_URL")
	origRegion := os.Getenv("AWS_REGION")
	origBucket := os.Getenv("S3_BUCKET")
	origKey := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	origSkipVerify := os.Getenv("STRESSER_INSECURE_SKIP_VERIFY")
	origOpType := os.Getenv("STRESSER_OPERATION_TYPE")
	origPutSize := os.Getenv("STRESSER_PUT_SIZE_KB")

	// Cleanup function to restore environment variables
	defer func() {
		os.Setenv("AWS_ENDPOINT_URL", origEndpoint)
		os.Setenv("AWS_REGION", origRegion)
		os.Setenv("S3_BUCKET", origBucket)
		os.Setenv("AWS_ACCESS_KEY_ID", origKey)
		os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)
		os.Setenv("STRESSER_INSECURE_SKIP_VERIFY", origSkipVerify)
		os.Setenv("STRESSER_OPERATION_TYPE", origOpType)
		os.Setenv("STRESSER_PUT_SIZE_KB", origPutSize)
	}()

	// Clear environment variables for clean test
	os.Unsetenv("AWS_ENDPOINT_URL")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("S3_BUCKET")
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	os.Unsetenv("STRESSER_INSECURE_SKIP_VERIFY")
	os.Unsetenv("STRESSER_OPERATION_TYPE")
	os.Unsetenv("STRESSER_PUT_SIZE_KB")

	// Test loading from file only
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify config values
	if cfg.Endpoint != "https://test-endpoint.com" {
		t.Errorf("Expected Endpoint='https://test-endpoint.com', got '%s'", cfg.Endpoint)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("Expected Region='us-east-1', got '%s'", cfg.Region)
	}
	if cfg.Bucket != "test-bucket" {
		t.Errorf("Expected Bucket='test-bucket', got '%s'", cfg.Bucket)
	}
	if cfg.AccessKey != "test-access-key" {
		t.Errorf("Expected AccessKey='test-access-key', got '%s'", cfg.AccessKey)
	}
	if cfg.SecretKey != "test-secret-key" {
		t.Errorf("Expected SecretKey='test-secret-key', got '%s'", cfg.SecretKey)
	}
	if cfg.OperationType != "mixed" {
		t.Errorf("Expected OperationType='mixed', got '%s'", cfg.OperationType)
	}
	if cfg.PutObjectSizeKB != 2048 {
		t.Errorf("Expected PutObjectSizeKB=2048, got %d", cfg.PutObjectSizeKB)
	}
	if !cfg.InsecureSkipVerify {
		t.Errorf("Expected InsecureSkipVerify=true, got %v", cfg.InsecureSkipVerify)
	}

	// Test environment variable override
	os.Setenv("AWS_ENDPOINT_URL", "https://env-endpoint.com")
	os.Setenv("AWS_REGION", "eu-west-1")
	os.Setenv("S3_BUCKET", "env-bucket")
	os.Setenv("AWS_ACCESS_KEY_ID", "env-access-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret-key")
	os.Setenv("STRESSER_INSECURE_SKIP_VERIFY", "false")
	os.Setenv("STRESSER_OPERATION_TYPE", "read")
	os.Setenv("STRESSER_PUT_SIZE_KB", "512")

	cfg, err = LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed with env vars: %v", err)
	}

	// Verify environment variables override file values
	if cfg.Endpoint != "https://env-endpoint.com" {
		t.Errorf("Expected Endpoint='https://env-endpoint.com', got '%s'", cfg.Endpoint)
	}
	if cfg.Region != "eu-west-1" {
		t.Errorf("Expected Region='eu-west-1', got '%s'", cfg.Region)
	}
	if cfg.Bucket != "env-bucket" {
		t.Errorf("Expected Bucket='env-bucket', got '%s'", cfg.Bucket)
	}
	if cfg.AccessKey != "env-access-key" {
		t.Errorf("Expected AccessKey='env-access-key', got '%s'", cfg.AccessKey)
	}
	if cfg.SecretKey != "env-secret-key" {
		t.Errorf("Expected SecretKey='env-secret-key', got '%s'", cfg.SecretKey)
	}
	if cfg.OperationType != "read" {
		t.Errorf("Expected OperationType='read', got '%s'", cfg.OperationType)
	}
	if cfg.PutObjectSizeKB != 512 {
		t.Errorf("Expected PutObjectSizeKB=512, got %d", cfg.PutObjectSizeKB)
	}
	if cfg.InsecureSkipVerify {
		t.Errorf("Expected InsecureSkipVerify=false, got %v", cfg.InsecureSkipVerify)
	}

	// Test applying flag values
	cfg.ApplyFlags("2m", 15, true, "flag-manifest.txt", "flag-output.csv", "write", 4096, 500, true)

	// Verify flag values override environment variables
	if cfg.Duration != "2m" {
		t.Errorf("Expected Duration='2m', got '%s'", cfg.Duration)
	}
	if cfg.Concurrency != 15 {
		t.Errorf("Expected Concurrency=15, got %d", cfg.Concurrency)
	}
	if !cfg.Randomize {
		t.Errorf("Expected Randomize=true, got %v", cfg.Randomize)
	}
	if cfg.ManifestPath != "flag-manifest.txt" {
		t.Errorf("Expected ManifestPath='flag-manifest.txt', got '%s'", cfg.ManifestPath)
	}
	if cfg.OutputFile != "flag-output.csv" {
		t.Errorf("Expected OutputFile='flag-output.csv', got '%s'", cfg.OutputFile)
	}
	if cfg.OperationType != "write" {
		t.Errorf("Expected OperationType='write', got '%s'", cfg.OperationType)
	}
	if cfg.PutObjectSizeKB != 4096 {
		t.Errorf("Expected PutObjectSizeKB=4096, got %d", cfg.PutObjectSizeKB)
	}
	if cfg.FileCount != 500 {
		t.Errorf("Expected FileCount=500, got %d", cfg.FileCount)
	}
	if !cfg.GenerateManifest {
		t.Errorf("Expected GenerateManifest=true, got %v", cfg.GenerateManifest)
	}
}
