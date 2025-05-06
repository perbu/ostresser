package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/perbu/ostresser/stresser"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

// --- Command Line Flags ---
var (
	// Configuration
	configPath = flag.String("config", "", "Path to YAML config file (optional, overrides env vars)")

	// Test Parameters
	duration    = flag.String("d", "1m", "Duration of the test (e.g., 30s, 5m, 1h)")
	concurrency = flag.Int("c", 10, "Number of concurrent workers")
	randomize   = flag.Bool("r", false, "Randomize access to keys in the manifest for READ ops (default: sequential)")
	opType      = flag.String("op", stresser.DefaultOperationType, "Operation type: 'read', 'write', or 'mixed'")
	putSizeKB   = flag.Int("putsize", stresser.DefaultPutSizeKB, "Size of objects to upload in KB for 'write' or 'mixed' mode")
	fileCount   = flag.Int("files", stresser.DefaultFileCount, "Number of files to generate for 'write' mode")
	genManifest = flag.Bool("genmf", true, "Generate manifest file with created objects in 'write' mode")

	// Output
	outputFile = flag.String("o", "stress_results.csv", "Output CSV file path for detailed results")

	// Logging
	logLevel = flag.String("log-level", stresser.DefaultLogLevel, "Log level: debug, info, warn, error")

	// Meta
	showVersion = flag.Bool("version", false, "Show version information and exit")
)

func main() {
	// Configure flag usage message
	info, _ := debug.ReadBuildInfo()
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <manifest.txt>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Object Store Stress Tester (Version: %q, Go: %q)\n\n", info.Main.Version, info.GoVersion)
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  <manifest.txt>   Path to the text file containing object keys (one per line).\n")
		fmt.Fprintf(os.Stderr, "                   Required for 'read' and 'mixed' modes. Ignored for 'write' mode.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfiguration Precedence: Flags > Environment Variables > YAML Config File\n")
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  AWS_ENDPOINT_URL, AWS_REGION, S3_BUCKET\n")
		fmt.Fprintf(os.Stderr, "  AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (or use default credential chain)\n")
		fmt.Fprintf(os.Stderr, "  STRESSER_OPERATION_TYPE ('read'|'write'|'mixed')\n")
		fmt.Fprintf(os.Stderr, "  STRESSER_PUT_SIZE_KB (integer)\n")
		fmt.Fprintf(os.Stderr, "  STRESSER_INSECURE_SKIP_VERIFY ('true'|'false')\n")
		fmt.Fprintf(os.Stderr, "  STRESSER_LOG_LEVEL ('debug'|'info'|'warn'|'error')\n")
	}

	// Parse command line flags
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("Version: %q, GO: %q)\n\n", info.Main.Version, info.GoVersion)
		os.Exit(0)
	}

	// Check for required manifest argument (conditionally required based on opType later)
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Error: Manifest file path argument is required.")
		flag.Usage()
		os.Exit(1)
	}
	manifestPath := flag.Arg(0)

	// --- Context Setup for Graceful Shutdown ---
	// Create a root context that listens for interrupt signals (Ctrl+C)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// Call stop() when main exits to release resources associated with signal listening
	defer stop()

	// --- Run the application logic ---
	// Keep main() minimal, delegate to run() function
	if err := run(ctx, manifestPath); err != nil {
		slog.Error("Error running stress test", "error", err)
		os.Exit(1)
	}

	slog.Info("Stress test completed successfully")
}

// run encapsulates the main application logic: config loading, validation, execution, reporting.
func run(ctx context.Context, manifestPath string) error {
	// 1. Load Configuration (from YAML and Env vars)
	cfg, err := stresser.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load base configuration: %w", err)
	}

	// 2. Apply Flag overrides to Config
	cfg.ApplyFlags(*duration, *concurrency, *randomize, manifestPath, *outputFile, *opType, *putSizeKB, *fileCount, *genManifest, *logLevel)

	// 3. Configure Logger based on Config
	setupLogger(cfg.LogLevel)

	// 4. Validate Final Configuration
	if err := cfg.Validate(); err != nil {
		// Provide usage context if validation fails
		flag.Usage()
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// 5. Execute the Stress Test
	slog.Info("Starting stress test run...",
		"duration", cfg.Duration,
		"concurrency", cfg.Concurrency,
		"operation", cfg.OperationType)

	results, stats, err := stresser.RunStressTest(ctx, cfg)
	if err != nil {
		// Check if the error was due to context cancellation (timeout or signal) - this is expected
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			slog.Info("Test run ended gracefully due to context cancellation", "reason", ctx.Err())
			// Proceed to report results collected so far
		} else {
			// A different, unexpected error occurred during the run
			return fmt.Errorf("stress test execution failed: %w", err)
		}
	}

	// Ensure stats are available even if the run was interrupted early
	if stats == nil {
		slog.Warn("Statistics object is nil, possibly due to early termination before workers started")
		stats = stresser.NewStats() // Create empty stats
		// Optionally try to calculate from partial results if available
		if len(results) > 0 {
			slog.Info("Attempting to calculate stats from partial results...")
			startTime := results[0].Timestamp // Approximate start
			endTime := time.Now()             // Approximate end
			for _, res := range results {
				stats.AddResult(res)
			}
			stats.Calculate(startTime, endTime)
		}
	}

	// 6. Print Summary Statistics to Console
	if stats != nil {
		stats.PrintSummary(os.Stdout)
	}

	// 7. Write Detailed Results to CSV
	if len(results) > 0 {
		if err := stresser.WriteResultsCSV(results, cfg.OutputFile); err != nil {
			// Log CSV writing error but don't necessarily fail the whole run
			slog.Error("Error writing results CSV", "error", err, "file", cfg.OutputFile)
			// return fmt.Errorf("failed to write results CSV: %w", err) // Optionally make this fatal
		}
	} else {
		slog.Warn("No results collected, skipping CSV output")
	}

	// If we reached here without returning an unexpected error from RunStressTest, it's a success.
	return nil
}

// setupLogger configures the slog logger based on the log level
func setupLogger(level string) {
	var logLevel slog.Level

	// Set log level based on configuration
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		// Default to info if invalid level provided
		logLevel = slog.LevelInfo
	}

	// Create a text-based handler with the configured level
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})

	// Set the default logger
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Debug("Logger initialized", "level", level)
}
