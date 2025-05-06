package stresser

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

// Result holds the metrics for a single S3 operation (GET or PUT).
type Result struct {
	Timestamp       time.Time
	Operation       string // "GET" or "PUT"
	ObjectKey       string
	TTFB            time.Duration // GET: Time To First Byte (proxy: time until headers received) | PUT: N/A (-1)
	TTLB            time.Duration // GET: Time To Last Byte (body read) | PUT: Time until PutObject returns
	BytesDownloaded int64         // Bytes read for GET
	BytesUploaded   int64         // Bytes written for PUT
	Error           string        // Empty if successful
}

// Stats aggregates results from multiple operations.
type Stats struct {
	TotalRequests  int64
	TotalGets      int64
	TotalPuts      int64
	TotalErrors    int64
	TotalBytesDown int64
	TotalBytesUp   int64
	Concurrency    int             // Number of concurrent workers used in the test
	GetTTFBs       []time.Duration // Latencies only for successful GETs
	GetTTLBs       []time.Duration // Latencies only for successful GETs
	PutTTLBs       []time.Duration // Latencies only for successful PUTs (TTLB represents full PUT duration)
	MinGetTTFB     time.Duration
	MaxGetTTFB     time.Duration
	AvgGetTTFB     time.Duration
	P50GetTTFB     time.Duration
	P90GetTTFB     time.Duration
	P99GetTTFB     time.Duration
	MinGetTTLB     time.Duration
	MaxGetTTLB     time.Duration
	AvgGetTTLB     time.Duration
	P50GetTTLB     time.Duration
	P90GetTTLB     time.Duration
	P99GetTTLB     time.Duration
	MinPutTTLB     time.Duration // Min time for a PUT operation
	MaxPutTTLB     time.Duration // Max time for a PUT operation
	AvgPutTTLB     time.Duration // Avg time for a PUT operation
	P50PutTTLB     time.Duration
	P90PutTTLB     time.Duration
	P99PutTTLB     time.Duration
	mu             sync.Mutex // Protects updates if AddResult were concurrent (currently sequential)
	startTime      time.Time
	endTime        time.Time
	actualDuration time.Duration
}

// NewStats initializes a Stats object.
func NewStats() *Stats {
	// Initialize Min values high and Max values low/negative for comparison
	largeDuration := time.Hour * 24
	return &Stats{
		GetTTFBs:   make([]time.Duration, 0),
		GetTTLBs:   make([]time.Duration, 0),
		PutTTLBs:   make([]time.Duration, 0),
		MinGetTTFB: largeDuration,
		MinGetTTLB: largeDuration,
		MinPutTTLB: largeDuration,
		MaxGetTTFB: -1,
		MaxGetTTLB: -1,
		MaxPutTTLB: -1,
	}
}

// AddResult incorporates a single result into the aggregate statistics.
// This should be called sequentially after all results are collected.
func (s *Stats) AddResult(r Result) {
	s.TotalRequests++
	isGet := r.Operation == "GET"
	isPut := r.Operation == "PUT"

	if isGet {
		s.TotalGets++
	} else if isPut {
		s.TotalPuts++
	}

	if r.Error != "" {
		s.TotalErrors++
		return // Don't include failed requests in latency/throughput stats
	}

	// Process successful requests
	if isGet {
		s.TotalBytesDown += r.BytesDownloaded
		s.GetTTFBs = append(s.GetTTFBs, r.TTFB)
		s.GetTTLBs = append(s.GetTTLBs, r.TTLB)

		if r.TTFB < s.MinGetTTFB {
			s.MinGetTTFB = r.TTFB
		}
		if r.TTFB > s.MaxGetTTFB {
			s.MaxGetTTFB = r.TTFB
		}
		if r.TTLB < s.MinGetTTLB {
			s.MinGetTTLB = r.TTLB
		}
		if r.TTLB > s.MaxGetTTLB {
			s.MaxGetTTLB = r.TTLB
		}
	} else if isPut {
		s.TotalBytesUp += r.BytesUploaded
		s.PutTTLBs = append(s.PutTTLBs, r.TTLB) // Use TTLB for PUT duration

		if r.TTLB < s.MinPutTTLB {
			s.MinPutTTLB = r.TTLB
		}
		if r.TTLB > s.MaxPutTTLB {
			s.MaxPutTTLB = r.TTLB
		}
	}
}

// Calculate computes final aggregate statistics like averages and percentiles.
func (s *Stats) Calculate(startTime, endTime time.Time) {
	s.startTime = startTime
	s.endTime = endTime
	s.actualDuration = endTime.Sub(startTime)

	// Reset unrealistic min/max if no successful operations of that type occurred
	largeDuration := time.Hour * 24
	if len(s.GetTTFBs) == 0 {
		if s.MinGetTTFB == largeDuration {
			s.MinGetTTFB = 0
		}
		if s.MaxGetTTFB == -1 {
			s.MaxGetTTFB = 0
		}
	}
	if len(s.GetTTLBs) == 0 {
		if s.MinGetTTLB == largeDuration {
			s.MinGetTTLB = 0
		}
		if s.MaxGetTTLB == -1 {
			s.MaxGetTTLB = 0
		}
	}
	if len(s.PutTTLBs) == 0 {
		if s.MinPutTTLB == largeDuration {
			s.MinPutTTLB = 0
		}
		if s.MaxPutTTLB == -1 {
			s.MaxPutTTLB = 0
		}
	}

	// Calculate GET stats
	if len(s.GetTTFBs) > 0 {
		sortDurations(s.GetTTFBs)
		sortDurations(s.GetTTLBs)
		s.AvgGetTTFB = averageDuration(s.GetTTFBs)
		s.AvgGetTTLB = averageDuration(s.GetTTLBs)
		s.P50GetTTFB = percentileDuration(s.GetTTFBs, 50)
		s.P90GetTTFB = percentileDuration(s.GetTTFBs, 90)
		s.P99GetTTFB = percentileDuration(s.GetTTFBs, 99)
		s.P50GetTTLB = percentileDuration(s.GetTTLBs, 50)
		s.P90GetTTLB = percentileDuration(s.GetTTLBs, 90)
		s.P99GetTTLB = percentileDuration(s.GetTTLBs, 99)
	}

	// Calculate PUT stats
	if len(s.PutTTLBs) > 0 {
		sortDurations(s.PutTTLBs)
		s.AvgPutTTLB = averageDuration(s.PutTTLBs)
		s.P50PutTTLB = percentileDuration(s.PutTTLBs, 50)
		s.P90PutTTLB = percentileDuration(s.PutTTLBs, 90)
		s.P99PutTTLB = percentileDuration(s.PutTTLBs, 99)
	}
}

// --- Helper functions for stats calculation ---

func sortDurations(data []time.Duration) {
	sort.Slice(data, func(i, j int) bool { return data[i] < data[j] })
}

func averageDuration(data []time.Duration) time.Duration {
	if len(data) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range data {
		total += d
	}
	return total / time.Duration(len(data))
}

func percentileDuration(sortedData []time.Duration, p int) time.Duration {
	if len(sortedData) == 0 {
		return 0
	}

	// For small datasets, use a more appropriate algorithm
	if len(sortedData) <= 2 {
		// For 1 element, return it
		if len(sortedData) == 1 {
			return sortedData[0]
		}
		// For 2 elements, P50 = first element, P90/P99 = second element
		if p <= 50 {
			return sortedData[0]
		}
		return sortedData[1]
	}

	// For larger datasets, use Nearest Rank method
	// index = ceil(P/100 * N) - 1
	index := (p * len(sortedData)) / 100
	if index < 0 {
		index = 0
	} // Ensure index is not negative
	if index >= len(sortedData) {
		index = len(sortedData) - 1
	} // Ensure index is within bounds
	return sortedData[index]
}

// PrintSummary prints the calculated statistics to the given writer.
func (s *Stats) PrintSummary(w io.Writer) {
	successGets := s.TotalGets - s.countErrorsForOp("GET") // Requires tracking errors per op or filtering results
	successPuts := s.TotalPuts - s.countErrorsForOp("PUT") // Placeholder - needs refinement if error counts per op needed
	totalSuccess := s.TotalRequests - s.TotalErrors

	throughputDownMBps := float64(0)
	throughputUpMBps := float64(0)
	requestsPerSec := float64(0)

	if s.actualDuration.Seconds() > 0 {
		requestsPerSec = float64(s.TotalRequests) / s.actualDuration.Seconds()
		throughputDownMBps = (float64(s.TotalBytesDown) / (1024 * 1024)) / s.actualDuration.Seconds()
		throughputUpMBps = (float64(s.TotalBytesUp) / (1024 * 1024)) / s.actualDuration.Seconds()
	}

	fmt.Fprintf(w, "\n--- Stress Test Summary --- (%s) ---\n", s.actualDuration.Round(time.Millisecond))
	fmt.Fprintf(w, "Overall:\n")
	fmt.Fprintf(w, "  Concurrency:    %d\n", s.Concurrency)
	fmt.Fprintf(w, "  Total Requests: %d (%.2f req/s)\n", s.TotalRequests, requestsPerSec)
	fmt.Fprintf(w, "  Total Success:  %d\n", totalSuccess)
	fmt.Fprintf(w, "  Total Errors:   %d\n", s.TotalErrors)
	fmt.Fprintf(w, "\nGET Operations (%d total):\n", s.TotalGets)
	fmt.Fprintf(w, "  Success:        %d\n", successGets) // Placeholder count
	fmt.Fprintf(w, "  Bytes D/L:      %d (%.2f MiB)\n", s.TotalBytesDown, float64(s.TotalBytesDown)/(1024*1024))
	fmt.Fprintf(w, "  Avg Throughput: %.2f MiB/s\n", throughputDownMBps)

	if successGets > 0 {
		fmt.Fprintf(w, "  Latency (ms): |   Min  |   Avg  |   P50  |   P90  |   P99  |   Max  \n")
		fmt.Fprintf(w, "  --------------|--------|--------|--------|--------|--------|--------\n")
		fmt.Fprintf(w, "  TTFB (proxy)  |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f \n",
			ms(s.MinGetTTFB), ms(s.AvgGetTTFB), ms(s.P50GetTTFB), ms(s.P90GetTTFB), ms(s.P99GetTTFB), ms(s.MaxGetTTFB))
		fmt.Fprintf(w, "  TTLB (body)   |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f \n",
			ms(s.MinGetTTLB), ms(s.AvgGetTTLB), ms(s.P50GetTTLB), ms(s.P90GetTTLB), ms(s.P99GetTTLB), ms(s.MaxGetTTLB))
	} else {
		fmt.Fprintln(w, "  No successful GETs to calculate latency.")
	}

	fmt.Fprintf(w, "\nPUT Operations (%d total):\n", s.TotalPuts)
	fmt.Fprintf(w, "  Success:        %d\n", successPuts) // Placeholder count
	fmt.Fprintf(w, "  Bytes U/L:      %d (%.2f MiB)\n", s.TotalBytesUp, float64(s.TotalBytesUp)/(1024*1024))
	if successPuts > 0 {
		avgObjectSizeKB := float64(s.TotalBytesUp) / float64(successPuts) / 1024
		fmt.Fprintf(w, "  Object Size:    %.2f KiB\n", avgObjectSizeKB)
	}
	fmt.Fprintf(w, "  Avg Throughput: %.2f MiB/s\n", throughputUpMBps)

	if successPuts > 0 {
		fmt.Fprintf(w, "  Latency (ms): |   Min  |   Avg  |   P50  |   P90  |   P99  |   Max  \n")
		fmt.Fprintf(w, "  --------------|--------|--------|--------|--------|--------|--------\n")
		fmt.Fprintf(w, "  TTLB (total)  |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f |%7.2f \n",
			ms(s.MinPutTTLB), ms(s.AvgPutTTLB), ms(s.P50PutTTLB), ms(s.P90PutTTLB), ms(s.P99PutTTLB), ms(s.MaxPutTTLB))
	} else {
		fmt.Fprintln(w, "  No successful PUTs to calculate latency.")
	}
	fmt.Fprintf(w, "----------------------------------------\n")
}

// Helper to count errors for a specific operation type (requires iterating results or storing counts)
// This is a placeholder - a more efficient approach might store error counts per type during AddResult
func (s *Stats) countErrorsForOp(opType string) int64 {
	// This requires access to the raw results, which are not stored in Stats currently.
	// For simplicity, returning 0. A real implementation would need modification.
	// Alternatively, calculate success counts directly in AddResult.
	// Let's recalculate success counts here based on totals for now
	if opType == "GET" {
		// Estimate: Total Errors might be distributed proportionally? Not accurate.
		// Best approach is to calculate success = total - errors during AddResult
		// Returning placeholder:
		return s.TotalGets - int64(len(s.GetTTLBs)) // Number of successful GETs is length of GetTTLBs
	}
	if opType == "PUT" {
		return s.TotalPuts - int64(len(s.PutTTLBs)) // Number of successful PUTs is length of PutTTLBs
	}
	return 0
}

// Helper to convert duration to milliseconds float
func ms(d time.Duration) float64 {
	if d < 0 { // Handle cases like uninitialized Max values
		return 0.0
	}
	return float64(d.Nanoseconds()) / 1e6
}

// WriteResultsCSV writes the collected results to a CSV file.
func WriteResultsCSV(results []Result, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output csv file %s: %w", filePath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush() // Ensure all buffered data is written

	// Write header
	header := []string{"Timestamp", "Operation", "ObjectKey", "TTFB(ms)", "TTLB(ms)", "BytesDownloaded", "BytesUploaded", "Error"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	// Write data rows
	for _, r := range results {
		row := []string{
			r.Timestamp.Format(time.RFC3339Nano),
			r.Operation,
			r.ObjectKey,
			fmt.Sprintf("%.3f", ms(r.TTFB)), // TTFB (ms) - will be 0.000 for PUTs or errors
			fmt.Sprintf("%.3f", ms(r.TTLB)), // TTLB (ms)
			fmt.Sprintf("%d", r.BytesDownloaded),
			fmt.Sprintf("%d", r.BytesUploaded),
			r.Error,
		}
		if err := writer.Write(row); err != nil {
			// Log error but attempt to continue writing other rows
			fmt.Fprintf(os.Stderr, "Warning: failed to write csv row: %v (data: %v)\n", err, row)
			// Decide whether to return immediately or try to continue
			// return fmt.Errorf("failed to write csv row: %w", err)
		}
	}

	// Check for errors that might have occurred during flushing
	if err := writer.Error(); err != nil {
		return fmt.Errorf("error during csv writing/flushing: %w", err)
	}

	fmt.Printf("Detailed results written to %s\n", filePath)
	return nil
}
