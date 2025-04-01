package stresser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand" // Use math/rand for all random operations
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// RunStressTest orchestrates the stress test, launching workers and collecting results.
func RunStressTest(ctx context.Context, cfg *Config) ([]Result, *Stats, error) {
	// 1. Load or prepare manifest
	var objectKeys []string
	var manifestWriter *ManifestWriter
	var err error

	// For read/mixed mode, load existing manifest
	if cfg.OperationType == "read" || cfg.OperationType == "mixed" {
		objectKeys, err = LoadManifest(cfg.ManifestPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load manifest for read/mixed mode: %w", err)
		}
		slog.Info("Loaded object keys from manifest", "count", len(objectKeys), "path", cfg.ManifestPath)
	} else if cfg.OperationType == "write" {
		// For write-only mode with file generation
		if cfg.GenerateManifest {
			manifestWriter, err = NewManifestWriter(cfg.ManifestPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create manifest writer: %w", err)
			}
			defer manifestWriter.Close()
			slog.Info("Will generate manifest file", "path", cfg.ManifestPath)
		} else {
			slog.Info("Write-only mode selected", "manifestGeneration", "disabled")
		}

		// If we're in write mode and want to pre-generate specific number of files instead of continuous generation
		if cfg.FileCount > 0 {
			slog.Info("Will generate and upload files", "count", cfg.FileCount, "sizeKB", cfg.PutObjectSizeKB)
		}
	}

	// 2. Create S3 Client
	s3Client, err := NewS3Client(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create S3 client: %w", err)
	}
	slog.Info("S3 client configured", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket)

	// 3. Setup Concurrency & Context with Timeout
	runDuration, err := time.ParseDuration(cfg.Duration)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid duration format %q: %w", cfg.Duration, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, runDuration)
	defer cancel() // Ensure cancellation propagates when RunStressTest returns

	resultsChan := make(chan Result, cfg.Concurrency*20) // Buffered channel
	var wg sync.WaitGroup

	// Each worker will generate its own unique PUT data to avoid object deduplication
	slog.Info("Workers will generate unique data for each PUT operation", "sizeKB", cfg.PutObjectSizeKB)

	slog.Info("Starting stress test",
		"concurrency", cfg.Concurrency,
		"duration", runDuration,
		"operation", cfg.OperationType,
		"randomizeRead", cfg.Randomize,
		"putSizeKB", cfg.PutObjectSizeKB)

	startTime := time.Now()

	// 4. Start Workers
	if cfg.OperationType == "write" && cfg.FileCount > 0 {
		// Use fixed file count generation approach
		wg.Add(1)
		go generateFiles(runCtx, &wg, s3Client, cfg, resultsChan, manifestWriter)
	} else {
		// Use traditional workers for continuous test
		for i := 0; i < cfg.Concurrency; i++ {
			wg.Add(1)
			// Pass runCtx which has the timeout
			go runWorker(runCtx, &wg, i, s3Client, cfg, objectKeys, resultsChan, manifestWriter)
		}
	}

	// 5. Wait for workers to finish and close results channel
	// This goroutine ensures close(resultsChan) happens *after* all workers signal Done.
	go func() {
		wg.Wait()
		close(resultsChan)
		slog.Info("All workers finished")
	}()

	// 6. Collect Results from the channel until it's closed
	allResults := make([]Result, 0)
	for result := range resultsChan {
		allResults = append(allResults, result)
		// Optional: Log progress periodically
		// if len(allResults)%100 == 0 { slog.Info("Collected results progress", "count", len(allResults)) }
	}
	endTime := time.Now()
	slog.Info("Collected total results", "count", len(allResults))

	// 7. Calculate Final Statistics
	stats := NewStats()
	for _, res := range allResults {
		stats.AddResult(res) // AddResult handles filtering successes/failures for stats
	}
	stats.Calculate(startTime, endTime) // Calculate averages, percentiles etc.

	// Check if the test ended due to timeout or external signal rather than an error
	if runCtx.Err() != nil && runCtx.Err() != context.Canceled && runCtx.Err() != context.DeadlineExceeded {
		// If the context error is something else, return it
		return allResults, stats, fmt.Errorf("test run context ended unexpectedly: %w", runCtx.Err())
	}
	// If context ended due to timeout/cancel, it's not an application error, return nil error.

	return allResults, stats, nil // Return collected results, stats, and nil error for normal completion/timeout
}

// runWorker performs S3 operations (GET, PUT, or mixed) until the context is cancelled.
func runWorker(ctx context.Context, wg *sync.WaitGroup, id int, s3Client S3ClientAPI, cfg *Config, objectKeys []string, resultsChan chan<- Result, manifestWriter *ManifestWriter) {
	defer wg.Done()
	slog.Info("Worker started", "id", id, "operation", cfg.OperationType)

	// Initialize random source per worker for non-crypto choices (key selection, op type in mixed mode)
	// Seed with unique value for each worker
	localRand := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))

	keyCount := len(objectKeys)       // Will be 0 in write-only mode
	keyIndex := id % max(keyCount, 1) // Simple initial distribution for sequential reads (if keyCount > 0)

	for {
		// Check for context cancellation *before* starting an operation
		select {
		case <-ctx.Done():
			slog.Info("Worker stopping", "id", id, "reason", ctx.Err())
			return // Context cancelled (timeout or external signal)
		default:
			// Continue processing
		}

		var result Result
		opType := cfg.OperationType

		// Decide operation type for 'mixed' mode
		if opType == "mixed" {
			if localRand.Intn(2) == 0 { // 50/50 chance
				opType = "read"
			} else {
				opType = "write"
			}
		}

		// Perform selected operation
		switch opType {
		case "read":
			if keyCount == 0 {
				slog.Warn("Skipping READ operation", "workerId", id, "reason", "no keys loaded (write-only mode or empty manifest)")
				// Avoid busy-looping if manifest is empty in read/mixed mode
				time.Sleep(100 * time.Millisecond) // Small delay
				continue
			}
			var objectKey string
			if cfg.Randomize {
				objectKey = objectKeys[localRand.Intn(keyCount)]
			} else {
				objectKey = objectKeys[keyIndex%keyCount]
				keyIndex++ // Only advance index for sequential reads
			}
			result = performGetOperation(ctx, s3Client, cfg.Bucket, objectKey)

		case "write":
			// Generate a unique key for each PUT to avoid overwrites (or use manifest keys if desired?)
			// Using unique keys is generally better for write stress tests.
			objectKey := fmt.Sprintf("stresser/worker%d/%d-%s.dat", id, time.Now().UnixNano(), randomString(8, localRand))

			// Generate unique data for each PUT to avoid object deduplication
			data := make([]byte, cfg.PutObjectSizeKB*1024)
			// Use math/rand which is faster and doesn't risk entropy exhaustion
			for i := range data {
				data[i] = byte(localRand.Intn(256))
			}

			result = performPutOperation(ctx, s3Client, cfg.Bucket, objectKey, data)

			// If successful upload and manifest writing is enabled, add the key to manifest
			if result.Error == "" && manifestWriter != nil {
				if err := manifestWriter.AddKey(objectKey); err != nil {
					slog.Error("Failed to write key to manifest", "workerId", id, "error", err)
				}
			}

		default:
			// Should not happen due to config validation, but handle defensively
			slog.Error("Invalid operation type encountered", "workerId", id, "operationType", opType)
			time.Sleep(time.Second) // Prevent fast loop on error
			continue
		}

		// Send result (even if it's an error result) to the collector
		// Non-blocking send attempt in case channel is full (shouldn't happen with sufficient buffer)
		select {
		case resultsChan <- result:
			// Result sent successfully
		case <-ctx.Done():
			// Context cancelled while trying to send, log and exit worker
			slog.Info("Context cancelled while sending result", "workerId", id, "reason", ctx.Err())
			return
		default:
			// Should ideally not happen with a buffered channel unless producer is way faster than consumer
			slog.Warn("Results channel potentially full, dropping result", "workerId", id, "key", result.ObjectKey)
		}
	}
}

// generateFiles generates and uploads a specific number of files, then exits.
// This is used for the fixed file count generation mode.
func generateFiles(ctx context.Context, wg *sync.WaitGroup, s3Client S3ClientAPI, cfg *Config, resultsChan chan<- Result, manifestWriter *ManifestWriter) {
	defer wg.Done()
	slog.Info("File generator started", "files", cfg.FileCount, "sizeKB", cfg.PutObjectSizeKB)

	// Initialize random source for key generation
	localRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Create files concurrently using a pool of workers
	filesChan := make(chan int, cfg.FileCount)
	var workerWg sync.WaitGroup

	// Fill the channel with file IDs
	for i := 0; i < cfg.FileCount; i++ {
		filesChan <- i
	}
	close(filesChan)

	// Use Concurrency workers to generate files in parallel
	for i := 0; i < cfg.Concurrency; i++ {
		workerWg.Add(1)
		go func(workerId int) {
			defer workerWg.Done()

			for fileId := range filesChan {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					slog.Info("Generator worker stopping", "workerId", workerId, "reason", ctx.Err())
					return
				default:
					// Continue processing
				}

				// Generate a unique key
				objectKey := fmt.Sprintf("stresser/generated/%d-%s.dat", fileId, randomString(8, localRand))

				// Generate unique data for each file to avoid object deduplication
				data := make([]byte, cfg.PutObjectSizeKB*1024)
				// Use math/rand which is faster and doesn't risk entropy exhaustion
				for i := range data {
					data[i] = byte(localRand.Intn(256))
				}

				// Upload the file with unique data
				result := performPutOperation(ctx, s3Client, cfg.Bucket, objectKey, data)

				// If successful upload and manifest writing is enabled, add the key to manifest
				if result.Error == "" && manifestWriter != nil {
					if err := manifestWriter.AddKey(objectKey); err != nil {
						slog.Error("Generator worker failed to write key to manifest", "workerId", workerId, "error", err)
					}
				}

				// Send result to result channel
				select {
				case resultsChan <- result:
					// Result sent successfully
				case <-ctx.Done():
					// Context cancelled while trying to send
					slog.Info("Generator worker context cancelled while sending result", "workerId", workerId, "reason", ctx.Err())
					return
				}

				// Log progress periodically
				if fileId > 0 && fileId%100 == 0 {
					slog.Info("Generated files progress", "current", fileId, "total", cfg.FileCount)
				}
			}
		}(i)
	}

	// Wait for all files to be generated
	workerWg.Wait()
	slog.Info("File generation completed", "files", cfg.FileCount)
}

// Helper function to avoid division by zero
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// performGetOperation executes a single S3 GET request and measures timing.
func performGetOperation(ctx context.Context, s3Client S3ClientAPI, bucket, key string) Result {
	result := Result{
		Timestamp: time.Now(),
		Operation: "GET",
		ObjectKey: key,
		TTFB:      -1, // Indicate not measured yet / error
		TTLB:      -1,
		Error:     "",
	}

	reqStartTime := time.Now()
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	// Perform the GetObject call
	resp, err := s3Client.GetObject(ctx, getObjectInput)
	timeHeadersReceived := time.Now() // Proxy for first byte (time GetObject returned)

	if err != nil {
		result.Error = err.Error()
		// slog.Debug("GET operation failed", "bucket", bucket, "key", key, "error", err) // Optional detailed logging
		return result // Return error result
	}
	// IMPORTANT: Ensure response body is closed even if errors occur later
	defer resp.Body.Close()

	// TTFB (Proxy): Duration until GetObject call returned successfully
	result.TTFB = timeHeadersReceived.Sub(reqStartTime)

	// Read the entire body to measure TTLB and BytesDownloaded
	// Using io.Copy is efficient for large files.
	bytesDownloaded, err := io.Copy(io.Discard, resp.Body) // Discard data, just count bytes & ensure it's read
	timeBodyRead := time.Now()

	if err != nil {
		// Error occurred while reading the body *after* headers were received
		result.Error = fmt.Sprintf("body read error: %v", err)
		result.BytesDownloaded = bytesDownloaded // Record bytes read before error
		// TTLB is duration until the error occurred during read
		result.TTLB = timeBodyRead.Sub(reqStartTime)
		// TTFB is still valid as headers were received
		return result
	}

	// TTLB: Duration until the entire body was successfully read
	result.TTLB = timeBodyRead.Sub(reqStartTime)
	result.BytesDownloaded = bytesDownloaded

	return result // Return success result
}

// performPutOperation executes a single S3 PUT request and measures timing.
func performPutOperation(ctx context.Context, s3Client S3ClientAPI, bucket, key string, data []byte) Result {
	result := Result{
		Timestamp: time.Now(),
		Operation: "PUT",
		ObjectKey: key,
		TTFB:      -1, // Not applicable for PUT in this context
		TTLB:      -1, // Will store total PUT duration
		Error:     "",
	}

	reqStartTime := time.Now()
	putObjectInput := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data), // Create a reader from the data slice
		// ContentLength: aws.Int64(int64(len(data))), // SDK often infers this, but explicit can be good
		// ContentType: aws.String("application/octet-stream"), // Optional: set content type
	}

	// Perform the PutObject call
	_, err := s3Client.PutObject(ctx, putObjectInput)
	timePutCompleted := time.Now()

	if err != nil {
		result.Error = err.Error()
		slog.Debug("PUT operation failed", "bucket", bucket, "key", key, "error", err)
		return result // Return error result
	}

	// TTLB for PUT represents the total time for the operation to complete
	result.TTLB = timePutCompleted.Sub(reqStartTime)
	result.BytesUploaded = int64(len(data))

	return result // Return success result
}

// randomString generates a random alphanumeric string of length n using the provided math/rand source.
func randomString(n int, r *rand.Rand) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}
