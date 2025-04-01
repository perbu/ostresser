package stresser

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	// S3 Connection
	Endpoint           string `yaml:"endpoint"`
	Region             string `yaml:"region"` // Needed for AWS SDK proper function even with custom endpoint
	Bucket             string `yaml:"bucket"`
	AccessKey          string `yaml:"accessKey"` // Optional if using env vars/instance profile
	SecretKey          string `yaml:"secretKey"` // Optional if using env vars/instance profile
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`

	// Test Parameters (populated from flags/args, overriding YAML/Env)
	Duration        string `yaml:"-"` // Exclude from YAML marshalling
	Concurrency     int    `yaml:"-"`
	Randomize       bool   `yaml:"-"`
	ManifestPath    string `yaml:"-"`
	OutputFile      string `yaml:"-"`
	OperationType   string `yaml:"operationType"`   // "read", "write", "mixed"
	PutObjectSizeKB int    `yaml:"putObjectSizeKB"` // Size in KB for PUT operations

	// File generation parameters for write mode
	FileCount        int  `yaml:"fileCount"`        // Number of files to generate in write mode (default: 1000)
	GenerateManifest bool `yaml:"generateManifest"` // Whether to write generated keys to manifest file

	// Logging configuration
	LogLevel string `yaml:"logLevel"` // Log level: debug, info, warn, error (default: info)
}

const (
	DefaultOperationType = "read"
	DefaultPutSizeKB     = 1024 // 1 MiB
	DefaultFileCount     = 1000 // Default number of files to generate
	DefaultLogLevel      = "info"
)

// LoadConfig loads configuration from a YAML file path or environment variables.
// Environment variables take precedence over YAML file values.
// Flags passed via command line override both YAML and environment variables.
func LoadConfig(configPath string) (*Config, error) {
	// Set defaults
	cfg := &Config{
		Region:           "us-east-1", // Default region if not specified
		OperationType:    DefaultOperationType,
		PutObjectSizeKB:  DefaultPutSizeKB,
		FileCount:        DefaultFileCount,
		GenerateManifest: true, // By default, generate manifest file when in write mode
		LogLevel:         DefaultLogLevel,
	}

	// 1. Load from YAML file if provided
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			// Don't fail if file doesn't exist, just log it maybe? Or let it proceed.
			// For now, fail if specified but unreadable.
			return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
		err = yaml.Unmarshal(data, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file %s: %w", configPath, err)
		}
	}

	// 2. Override with environment variables
	if envEndpoint := os.Getenv("AWS_ENDPOINT_URL"); envEndpoint != "" {
		cfg.Endpoint = envEndpoint
	}
	if envRegion := os.Getenv("AWS_REGION"); envRegion != "" {
		cfg.Region = envRegion
	}
	if envBucket := os.Getenv("S3_BUCKET"); envBucket != "" { // Using S3_BUCKET to avoid clash with AWS CLI profile buckets
		cfg.Bucket = envBucket
	}
	if envKey := os.Getenv("AWS_ACCESS_KEY_ID"); envKey != "" {
		cfg.AccessKey = envKey
	}
	if envSecret := os.Getenv("AWS_SECRET_ACCESS_KEY"); envSecret != "" {
		cfg.SecretKey = envSecret
	}

	// Handle boolean environment variables
	if skipVerify := os.Getenv("STRESSER_INSECURE_SKIP_VERIFY"); skipVerify != "" {
		// Only set to true if explicitly "true", otherwise set to false
		if skipVerify == "true" {
			cfg.InsecureSkipVerify = true
		} else if skipVerify == "false" {
			cfg.InsecureSkipVerify = false
		}
	}

	if envOpType := os.Getenv("STRESSER_OPERATION_TYPE"); envOpType != "" {
		cfg.OperationType = envOpType
	}
	if envPutSize := os.Getenv("STRESSER_PUT_SIZE_KB"); envPutSize != "" {
		var size int
		if _, err := fmt.Sscan(envPutSize, &size); err == nil && size > 0 {
			cfg.PutObjectSizeKB = size
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Invalid STRESSER_PUT_SIZE_KB value '%s', using default %d KB\n", envPutSize, DefaultPutSizeKB)
		}
	}
	if envFileCount := os.Getenv("STRESSER_FILE_COUNT"); envFileCount != "" {
		var count int
		if _, err := fmt.Sscan(envFileCount, &count); err == nil && count > 0 {
			cfg.FileCount = count
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Invalid STRESSER_FILE_COUNT value '%s', using default %d\n", envFileCount, DefaultFileCount)
		}
	}

	// Handle boolean for generate manifest
	if genManifest := os.Getenv("STRESSER_GENERATE_MANIFEST"); genManifest != "" {
		if genManifest == "true" {
			cfg.GenerateManifest = true
		} else if genManifest == "false" {
			cfg.GenerateManifest = false
		}
	}

	// Handle log level environment variable
	if logLevel := os.Getenv("STRESSER_LOG_LEVEL"); logLevel != "" {
		// Validate the log level
		switch strings.ToLower(logLevel) {
		case "debug", "info", "warn", "error":
			cfg.LogLevel = strings.ToLower(logLevel)
		default:
			fmt.Fprintf(os.Stderr, "Warning: Invalid STRESSER_LOG_LEVEL value '%s', using default '%s'\n", logLevel, DefaultLogLevel)
		}
	}

	// Basic validation (before applying flags)
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint URL is required (set via -config file, AWS_ENDPOINT_URL env var)")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required (set via -config file, S3_BUCKET env var)")
	}
	// Note: AccessKey/SecretKey might not be required if using EC2 instance profiles, etc.
	// SDK handles this, so we don't enforce it here.

	return cfg, nil
}

// ApplyFlags overrides config values with those provided by command-line flags.
func (c *Config) ApplyFlags(duration string, concurrency int, randomize bool, manifestPath, outputFile, opType string, putSizeKB int, fileCount int, generateManifest bool, logLevel string) {
	c.Duration = duration
	c.Concurrency = concurrency
	c.Randomize = randomize
	c.ManifestPath = manifestPath
	c.OutputFile = outputFile
	// Only override if the flag was actually set (or use its default if different from config default)
	if opType != DefaultOperationType {
		c.OperationType = opType
	}
	if putSizeKB != DefaultPutSizeKB && putSizeKB > 0 {
		c.PutObjectSizeKB = putSizeKB
	}
	if fileCount != DefaultFileCount && fileCount > 0 {
		c.FileCount = fileCount
	}
	c.GenerateManifest = generateManifest

	// Only override if a valid log level was specified
	if logLevel != DefaultLogLevel {
		// Validate the log level
		switch strings.ToLower(logLevel) {
		case "debug", "info", "warn", "error":
			c.LogLevel = strings.ToLower(logLevel)
		}
	}
}

// Validate ensures the final configuration (after flags) is valid.
func (c *Config) Validate() error {
	// Required fields from flags/args
	if c.Duration == "" {
		return fmt.Errorf("duration (-d) is required")
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency (-c) must be greater than 0")
	}
	if c.ManifestPath == "" {
		return fmt.Errorf("manifest file path argument is required")
	}
	if c.OutputFile == "" {
		return fmt.Errorf("output csv file path (-o) is required")
	}

	// Validate OperationType
	opLower := strings.ToLower(c.OperationType)
	switch opLower {
	case "read", "write", "mixed":
		c.OperationType = opLower // Normalize
	default:
		return fmt.Errorf("invalid operation type (-op): %s. Must be 'read', 'write', or 'mixed'", c.OperationType)
	}

	// Validate PutObjectSizeKB if relevant
	if c.OperationType == "write" || c.OperationType == "mixed" {
		if c.PutObjectSizeKB <= 0 {
			return fmt.Errorf("put object size (-putsize) must be greater than 0 KB for 'write' or 'mixed' mode")
		}
	}

	return nil
}
