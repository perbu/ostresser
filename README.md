# Object Store Stress Tester - Developer Guide

[![License](https://img.shields.io/badge/License-BSD_2--Clause-orange.svg)](https://opensource.org/licenses/BSD-2-Clause)

This document provides developer-specific information about the Go object store stress tester, focusing on its
configuration, programmatic usage, and internal design.

## Manifest File (manifest.txt)

The manifest file provides a list of object keys that the stress tester will interact with during 'read' or 'mixed'
operations.

Format:
* Plain text file.
* Each line contains one object key.
* Leading/trailing whitespace around keys is automatically trimmed.
* Empty lines are ignored.
* Lines containing only whitespace are ignored.


Example (manifest.txt):
```text


path/to/object1.dat
images/archive.zip
another/key/with/leading/space.txt
videos/important_video.mp4

backup-2024-01-15.tar.gz
trailing-space-key.log 
```

### Write Mode and Manifest File

In write mode, there are two ways to use the manifest file:

1. **Continuous Generation**: The stress tester will continuously generate and upload objects with random keys for the duration of the test.
   - The manifest file path is still required as a command-line argument.
   - By default, it will write all successfully uploaded object keys to the manifest file.
   - Use `-genmf=false` to disable writing to the manifest file.

2. **Fixed File Count Generation**: You can generate a specific number of files using the `-files` flag.
   - Example: `-files 1000` will generate 1000 files with random keys.
   - By default, all successfully uploaded keys will be written to the manifest file.
   - The test will exit after all files have been generated and uploaded.
   - File size is controlled with the `-putsize` flag (in KB).

Programmatic Usage (within the same module)

While the tool is primarily designed as a command-line application, its core logic in the internal/stresser package can
be invoked programmatically from other Go code within the same module.

Steps:

Create Configuration: Instantiate and populate the stresser.Config struct. You can manually set fields instead of
relying on file/env/flag parsing.

Create Context: Set up a context.Context, potentially with a timeout or cancellation signal.

Call RunStressTest: Invoke the main execution function, passing the context and config.

Process Results: Handle the returned results slice ([]stresser.Result) and statistics (*stresser.Stats).

Example Snippet:

package main // Or your package

import (
"context"
"fmt"
"log"
"os"
"time"

// Assuming your module path allows access or you adjust the structure
"[example.com/stress-tester/internal/stresser](https://www.google.com/search?q=https://example.com/stress-tester/internal/stresser)" //
Adjust import path

)

func runProgrammatically() {
// 1. Configure the test manually
cfg := &stresser.Config{
// Connection Details
Endpoint:           "http://localhost:9000", // Example endpoint
Region:             "us-east-1",
Bucket:             "my-test-bucket",
AccessKey:          "minioadmin", // Example credentials
SecretKey:          "minioadmin",
InsecureSkipVerify: true, // Example: Allow self-signed certs

	// Test Parameters (set directly)
	Duration:        "15s", // Use string format expected by time.ParseDuration internally
	Concurrency:     5,
	Randomize:       true,
	ManifestPath:    "path/to/your/manifest.txt", // Still needed for read/mixed
	OutputFile:      "programmatic_results.csv",  // Where to save CSV
	OperationType:   "mixed",                     // "read", "write", or "mixed"
	PutObjectSizeKB: 256,                         // 256 KB uploads

}

// Basic validation after manual setup
if err := cfg.Validate(); err != nil {
log.Fatalf("Manual configuration validation failed: %v", err)
}

// 2. Create a context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Overall timeout slightly longer than test
duration
defer cancel()

log.Println("Starting programmatic stress test run...")

// 3. Run the stress test
results, stats, err := stresser.RunStressTest(ctx, cfg)
if err != nil {
// Handle potential errors (excluding expected context cancellations)
if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
log.Printf("Test run ended via context: %v", ctx.Err())
} else {
log.Fatalf("Stress test execution failed: %v", err)
}
}

log.Printf("Programmatic run finished. Collected %d results.", len(results))

// 4. Process results and stats (example: print summary)
if stats != nil {
stats.PrintSummary(os.Stdout) // Print summary to console
} else {
log.Println("Stats object is nil.")
}

// You can also iterate through 'results' or save the CSV manually if needed
if len(results) > 0 {
// Example: Manually trigger CSV write if needed (RunStressTest usually does this)
// if err := stresser.WriteResultsCSV(results, cfg.OutputFile); err != nil {
// log.Printf("Error writing results CSV: %v", err)
// }
// log.Printf("First result TTFB (ms): %.3f", stresser.ms(results[0].TTFB)) // Requires helper func
} else {
log.Println("No results collected.")
}

}

// Helper function ms (needed if calling PrintSummary or using metrics directly)
// func ms(d time.Duration) float64 {
// if d < 0 { return 0.0 }
// return float64(d.Nanoseconds()) / 1e6
// }

Note on internal: Go's convention prevents packages inside internal from being imported by other modules. The example
above assumes usage within the same module. If you intend to use this stress tester as a library dependency in a
separate project, consider moving the relevant packages (like stresser) out of the internal directory.

Internals Overview (internal/stresser/)

The core logic resides within the internal/stresser package, broken down as follows:

config.go: Handles loading and validation of configuration from YAML files, environment variables, and command-line
flags. Defines the Config struct.

manifest.go: Contains the LoadManifest function responsible for reading and parsing the object key list from the
manifest file.

metrics.go: Defines the Result struct (holding metrics for a single operation) and the Stats struct (for aggregating
results). Includes functions for calculating summary statistics (Calculate, PrintSummary) and writing detailed results
to CSV (WriteResultsCSV).

s3client.go: Responsible for creating and configuring the AWS S3 client (*s3.Client) based on the application Config.
Handles endpoint resolution, credentials, and TLS settings. Defines the S3ClientAPI interface for mockability.

stresser.go: Contains the main orchestration logic (RunStressTest), the worker goroutine implementation (runWorker), and
the functions performing the actual S3 GET/PUT operations (performGetOperation, performPutOperation) while measuring
timings. Manages concurrency, context handling, and result collection.

The cmd/stress-tester/main.go file serves as the command-line entry point, responsible only for parsing flags, setting
up the initial context, calling stresser.RunStressTest, and handling top-level errors.