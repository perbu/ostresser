# Object Store Stress Tester - Developer Guide

[![License](https://img.shields.io/badge/License-BSD_2--Clause-orange.svg)](https://opensource.org/licenses/BSD-2-Clause)

This document provides developer-specific information about the Go object store stress tester, focusing on its
configuration, programmatic usage, and internal design.

## Install
```bash
go install github.com/perbu/ostresser@latest
```


## Example output.

```text
--- Stress Test Summary --- (7.556s) ---
Overall:
  Total Requests: 10000 (1323.46 req/s)
  Total Success:  10000
  Total Errors:   0

GET Operations (0 total):
  Success:        0
  Bytes D/L:      0 (0.00 MiB)
  Avg Throughput: 0.00 MiB/s
  No successful GETs to calculate latency.

PUT Operations (10000 total):
  Success:        10000
  Bytes U/L:      10485760000 (10000.00 MiB)
  Avg Throughput: 1323.46 MiB/s
  Latency (ms): |   Min  |   Avg  |   P50  |   P90  |   P99  |   Max
  --------------|--------|--------|--------|--------|--------|--------
  TTLB (total)  |   3.21 |   7.50 |   6.18 |  11.59 |  27.16 |  84.92
----------------------------------------
Detailed results written to stress_results.csv
time=2025-04-01T14:00:16.933+02:00 level=INFO msg="Stress test completed successfully"
```

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

1. **Continuous Generation**: The stress tester will continuously generate and upload objects with random keys for the
   duration of the test.
    - The manifest file path is still required as a command-line argument.
    - By default, it will write all successfully uploaded object keys to the manifest file.
    - Use `-genmf=false` to disable writing to the manifest file.

2. **Fixed File Count Generation**: You can generate a specific number of files using the `-files` flag.
    - Example: `-files 1000` will generate 1000 files with random keys.
    - By default, all successfully uploaded keys will be written to the manifest file.
    - The test will exit after all files have been generated and uploaded.
    - File size is controlled with the `-putsize` flag (in KB).

## Configuration options

### 1. S3 Connection Details

These parameters define how to connect to the S3-compatible object storage service.

* **`endpoint` (YAML) / `AWS_ENDPOINT_URL` (Env)**
   * **Description:** The full URL of the S3-compatible endpoint (e.g., `http://localhost:9000` or `https://s3.amazonaws.com`).
   * **Required:** Yes (must be set via YAML or Environment Variable).
   * **Type:** `string`

* **`region` (YAML) / `AWS_REGION` (Env)**
   * **Description:** The AWS region associated with the endpoint. This is often required by the AWS SDK for proper signing and functioning, even when using a non-AWS S3-compatible endpoint.
   * **Required:** No (Defaults to `us-east-1` if not set).
   * **Type:** `string`
   * **Default:** `us-east-1`

* **`bucket` (YAML) / `S3_BUCKET` (Env)**
   * **Description:** The name of the S3 bucket to target for the stress test operations. Note the specific environment variable `S3_BUCKET` is used.
   * **Required:** Yes (must be set via YAML or Environment Variable).
   * **Type:** `string`

* **`accessKey` (YAML) / `AWS_ACCESS_KEY_ID` (Env)**
   * **Description:** The access key credential for authenticating with the S3 service.
   * **Required:** No (Optional. If not provided, the SDK may attempt to use other credential sources like IAM instance profiles or shared credential files).
   * **Type:** `string`

* **`secretKey` (YAML) / `AWS_SECRET_ACCESS_KEY` (Env)**
   * **Description:** The secret key credential for authenticating with the S3 service.
   * **Required:** No (Optional, typically provided alongside `accessKey`).
   * **Type:** `string`

* **`insecureSkipVerify` (YAML) / `STRESSER_INSECURE_SKIP_VERIFY` (Env)**
   * **Description:** If set to `true`, TLS certificate verification for the S3 endpoint will be skipped. Use with caution, primarily for testing with self-signed certificates. The environment variable must be set to the string `"true"` or `"false"`.
   * **Required:** No (Defaults to `false`).
   * **Type:** `bool`
   * **Default:** `false`

---

### 2. Test Parameters

These parameters control the execution of the stress test itself. Most are primarily set via command-line flags/arguments.

* **`Duration` (Flag `-d`)**
   * **Description:** Specifies the total duration the stress test should run (e.g., "10s", "5m", "1h").
   * **Required:** Yes (must be set via flag).
   * **Type:** `string` (parsed into a duration)
   * **Source:** Command-line flag (`-d`) only.

* **`Concurrency` (Flag `-c`)**
   * **Description:** The number of concurrent workers (goroutines) performing S3 operations.
   * **Required:** Yes (must be set via flag and be > 0).
   * **Type:** `int`
   * **Source:** Command-line flag (`-c`) only.

* **`Randomize` (Flag `--randomize`)**
   * **Description:** If set, the order of keys read from the manifest file will be randomized for each worker. If false, each worker processes a distinct, sequential chunk of the manifest.
   * **Required:** No (Defaults to `false` likely, based on typical flag handling).
   * **Type:** `bool`
   * **Source:** Command-line flag (`--randomize`) only.

* **`ManifestPath` (Positional Argument)**
   * **Description:** The path to the input manifest file. This file should contain a list of object keys (one per line) to be used for `read` or `mixed` operations. In `write` mode, if `generateManifest` is true, this is the *output* path where generated keys will be written.
   * **Required:** Yes (must be provided as a command-line argument).
   * **Type:** `string`
   * **Source:** Command-line argument only.

* **`OutputFile` (Flag `-o`)**
   * **Description:** The path to the CSV file where the results (performance metrics) of the stress test will be written.
   * **Required:** Yes (must be set via flag).
   * **Type:** `string`
   * **Source:** Command-line flag (`-o`) only.

* **`OperationType` (Flag `-op`, YAML `operationType`, Env `STRESSER_OPERATION_TYPE`)**
   * **Description:** Specifies the type of S3 operations to perform. Valid values are `"read"` (GET objects), `"write"` (PUT objects), or `"mixed"` (both GET and PUT objects). Values are case-insensitive but normalized to lowercase.
   * **Required:** No (Defaults to `read`).
   * **Type:** `string`
   * **Valid Values:** `read`, `write`, `mixed`
   * **Default:** `read`

* **`PutObjectSizeKB` (Flag `-putsize`, YAML `putObjectSizeKB`, Env `STRESSER_PUT_SIZE_KB`)**
   * **Description:** The size (in Kilobytes) of the objects to create when the `operationType` is `"write"` or `"mixed"`. Must be greater than 0 in these modes.
   * **Required:** Yes, if `operationType` is `write` or `mixed`.
   * **Type:** `int`
   * **Default:** `1024` (1 MiB)

---

### 3. File Generation Parameters (Write Mode)

These parameters are specifically used when `operationType` is set to `write`.

* **`FileCount` (Flag `-filecount`, YAML `fileCount`, Env `STRESSER_FILE_COUNT`)**
   * **Description:** The number of unique object keys (and thus files) to generate and potentially upload if running in `write` mode. This is used to determine how many PUT operations to attempt if generating data.
   * **Required:** No (Defaults to `1000`).
   * **Type:** `int`
   * **Default:** `1000`

* **`GenerateManifest` (Flag `--generate-manifest / --no-generate-manifest` , YAML `generateManifest`, Env `STRESSER_GENERATE_MANIFEST`)**
   * **Description:** Controls whether the list of generated object keys (determined by `FileCount`) should be written to the file specified by `ManifestPath` when running in `write` mode. The environment variable must be set to the string `"true"` or `"false"`. Flags likely control this boolean directly.
   * **Required:** No (Defaults to `true`).
   * **Type:** `bool`
   * **Default:** `true`

---

### 4. Logging Configuration

* **`LogLevel` (Flag `-loglevel`, YAML `logLevel`, Env `STRESSER_LOG_LEVEL`)**
   * **Description:** Controls the verbosity of the application's logging output. Valid values are `"debug"`, `"info"`, `"warn"`, `"error"`. Values are case-insensitive but normalized to lowercase.
   * **Required:** No (Defaults to `info`).
   * **Type:** `string`
   * **Valid Values:** `debug`, `info`, `warn`, `error`
   * **Default:** `info`



## Programmatic Usage (within the same module)

While the tool is primarily designed as a command-line application, its core logic in the internal/stresser package can
be invoked programmatically from other Go code within the same module.

Steps:

* Create Configuration: Instantiate and populate the stresser.Config struct. You can manually set fields instead of
  relying on file/env/flag parsing.
* Create Context: Set up a context.Context, potentially with a timeout or cancellation signal.
* Call RunStressTest: Invoke the main execution function, passing the context and config.
* Process Results: Handle the returned results slice ([]stresser.Result) and statistics (*stresser.Stats).

Example Snippet:

```go
package main // Or your package

import (
   "context"
   "log"
   "os"
   "time"

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

``` 

