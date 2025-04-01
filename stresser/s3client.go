package stresser

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3ClientAPI defines the interface for the S3 operations we need.
// This helps in mocking for tests.
type S3ClientAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	// Add other S3 operations here if needed (e.g., DeleteObject, HeadObject)
}

// NewS3Client creates a new S3 client configured according to the application config.
func NewS3Client(ctx context.Context, cfg *Config) (*s3.Client, error) {

	// --- Custom HTTP Client Setup ---
	// Allows for options like disabling TLS verification (use cautiously!)
	httpClient := &http.Client{}
	if cfg.InsecureSkipVerify {
		slog.Warn("Disabling TLS certificate verification for S3 client")
		// Clone default transport to avoid modifying global state
		customTransport := http.DefaultTransport.(*http.Transport).Clone()
		customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		httpClient.Transport = customTransport
	}

	// --- AWS SDK Configuration Options ---
	var sdkOpts []func(*config.LoadOptions) error

	// 1. Region
	sdkOpts = append(sdkOpts, config.WithRegion(cfg.Region))

	// 2. Custom Endpoint Resolver (Forces SDK to use the specified endpoint)
	if cfg.Endpoint != "" {
		endpointResolver := aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				// Return the custom endpoint configuration
				return aws.Endpoint{
					URL:               cfg.Endpoint,
					HostnameImmutable: true, // Crucial for non-AWS S3 services
					Source:            aws.EndpointSourceCustom,
				}, nil
			})
		sdkOpts = append(sdkOpts, config.WithEndpointResolverWithOptions(endpointResolver))
	}
	// 3. Custom HTTP Client
	sdkOpts = append(sdkOpts, config.WithHTTPClient(httpClient))

	// 4. Credentials Provider
	// Use static credentials ONLY if both key and secret are provided in config.
	// Otherwise, let the SDK's default credential chain handle it (env vars, shared config, IAM role).
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		staticProvider := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
		sdkOpts = append(sdkOpts, config.WithCredentialsProvider(staticProvider))
		slog.Info("Using static credentials provided in configuration")
	} else {
		slog.Info("Using default AWS credential chain (environment variables, shared config, IAM role, etc.)")
		// No need to explicitly add default provider, LoadDefaultConfig does this.
	}

	// --- Load AWS Configuration ---
	awsCfg, err := config.LoadDefaultConfig(ctx, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS SDK config: %w", err)
	}

	// --- Create S3 Client ---
	// UsePathStyle is often required for S3-compatible storage like MinIO or Ceph.
	// It might need to be configurable depending on the target system.
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // Force path-style addressing
		// Consider adding o.RetryMaxAttempts or other retry options if needed
	})
	slog.Info("S3 client created successfully", "endpoint", cfg.Endpoint, "region", cfg.Region, "user", cfg.AccessKey, "bucket", cfg.Bucket)

	return s3Client, nil
}
