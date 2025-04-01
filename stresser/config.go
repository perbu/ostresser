package stresser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
}

const (
	DefaultOperationType = "read"
	DefaultPutSizeKB     = 1024 // 1 MiB
)

// LoadConfig loads configuration from a YAML file path or environment variables.
// Environment variables take precedence over YAML file values.
// Flags passed via command line override both YAML and environment variables.
func LoadConfig(configPath string) (*Config, error) {
	// Set defaults
	cfg := &Config{
		Region:          "us-east-1", // Default region if not specified
		OperationType:   DefaultOperationType,
		PutObjectSizeKB: DefaultPutSizeKB,
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
	if os.Getenv("STRESSER_INSECURE_SKIP_VERIFY") == "true" {
		cfg.InsecureSkipVerify = true
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
func (c *Config) ApplyFlags(duration string, concurrency int, randomize bool, manifestPath, outputFile, opType string, putSizeKB int) {
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
