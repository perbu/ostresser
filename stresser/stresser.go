package stresser

import (
	"bytes"
	"context"
	"crypto/rand" // Use crypto/rand for better random data generation
	"fmt"
	"io"
	"log"
	mathrand "math/rand" // Use math/rand for non-crypto random choices (like picking keys)
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// RunStressTest orchestrates the stress test, launching workers and collecting results.
func RunStressTest(ctx context.Context, cfg *Config) ([]Result, *Stats, error) {
	// 1. Load Manifest (only strictly needed for 'read' or 'mixed' modes if reading existing keys)
	var objectKeys []string
	var err error
	if cfg.OperationType == "read" || cfg.OperationType == "mixed" {
		objectKeys, err = LoadManifest(cfg.ManifestPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load manifest for read/mixed mode: %w", err)
		}
		log.Printf("Loaded %d object keys from manifest %s", len(objectKeys), cfg.ManifestPath)
	} else {
		log.Printf("Write-only mode selected, manifest file '%s' will not be read, new keys will be generated.", cfg.ManifestPath)
	}

	// 2. Create S3 Client
	s3Client, err := NewS3Client(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create S3 client: %w", err)
	}
	log.Printf("S3 client configured for endpoint: %s, bucket: %s", cfg.Endpoint, cfg.Bucket)

	// 3. Setup Concurrency & Context with Timeout
	runDuration, err := time.ParseDuration(cfg.Duration)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid duration format %q: %w", cfg.Duration, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, runDuration)
	defer cancel() // Ensure cancellation propagates when RunStressTest returns

	resultsChan := make(chan Result, cfg.Concurrency*2) // Buffered channel
	var wg sync.WaitGroup

	// Prepare shared data buffer for PUT operations if needed
	var putData []byte
	if cfg.OperationType == "write" || cfg.OperationType == "mixed" {
		putData = make([]byte, cfg.PutObjectSizeKB*1024)
		_, err := rand.Read(putData) // Fill with crypto-random data
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate random data for PUT: %w", err)
		}
		log.Printf("Prepared %d KB data buffer for PUT operations.", cfg.PutObjectSizeKB)
	}

	log.Printf("Starting stress test: Concurrency=%d, Duration=%s, Operation=%s, RandomizeRead=%t, PutSizeKB=%d",
		cfg.Concurrency, runDuration, cfg.OperationType, cfg.Randomize, cfg.PutObjectSizeKB)

	startTime := time.Now()

	// 4. Start Workers
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		// Pass runCtx which has the timeout
		go runWorker(runCtx, &wg, i, s3Client, cfg, objectKeys, putData, resultsChan)
	}

	// 5. Wait for workers to finish and close results channel
	// This goroutine ensures close(resultsChan) happens *after* all workers signal Done.
	go func() {
		wg.Wait()
		close(resultsChan)
		log.Println("All workers finished.")
	}()

	// 6. Collect Results from the channel until it's closed
	allResults := make([]Result, 0)
	for result := range resultsChan {
		allResults = append(allResults, result)
		// Optional: Log progress periodically
		// if len(allResults)%100 == 0 { log.Printf("Collected %d results...", len(allResults)) }
	}
	endTime := time.Now()
	log.Printf("Collected %d total results.", len(allResults))

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
func runWorker(ctx context.Context, wg *sync.WaitGroup, id int, s3Client S3ClientAPI, cfg *Config, objectKeys []string, putData []byte, resultsChan chan<- Result) {
	defer wg.Done()
	log.Printf("Worker %d started (Op: %s)", id, cfg.OperationType)

	// Initialize random source per worker for non-crypto choices (key selection, op type in mixed mode)
	// Seed with unique value for each worker
	localRand := mathrand.New(mathrand.NewSource(time.Now().UnixNano() + int64(id)))

	keyCount := len(objectKeys) // Will be 0 in write-only mode
	keyIndex := id % keyCount   // Simple initial distribution for sequential reads (if keyCount > 0)

	for {
		// Check for context cancellation *before* starting an operation
		select {
		case <-ctx.Done():
			log.Printf("Worker %d stopping: %v", id, ctx.Err())
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
				log.Printf("Worker %d: Skipping READ op as no keys loaded (write-only mode or empty manifest?)", id)
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
			result = performPutOperation(ctx, s3Client, cfg.Bucket, objectKey, putData)

		default:
			// Should not happen due to config validation, but handle defensively
			log.Printf("Worker %d: Invalid operation type '%s' encountered in loop.", id, opType)
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
			log.Printf("Worker %d: Context cancelled while sending result: %v", id, ctx.Err())
			return
		default:
			// Should ideally not happen with a buffered channel unless producer is way faster than consumer
			log.Printf("Worker %d: Warning - Results channel potentially full. Dropping result for key %s.", id, result.ObjectKey)
		}
	}
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
		// log.Printf("DEBUG: GET %s/%s failed: %v", bucket, key, err) // Optional detailed logging
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
		// log.Printf("DEBUG: PUT %s/%s failed: %v", bucket, key, err) // Optional detailed logging
		return result // Return error result
	}

	// TTLB for PUT represents the total time for the operation to complete
	result.TTLB = timePutCompleted.Sub(reqStartTime)
	result.BytesUploaded = int64(len(data))

	return result // Return success result
}

// randomString generates a random alphanumeric string of length n using the provided math/rand source.
func randomString(n int, r *mathrand.Rand) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}
