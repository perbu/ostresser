package stresser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatsAddAndCalculate(t *testing.T) {
	stats := NewStats()

	// Create some test results
	now := time.Now()

	// Successful GET operations
	stats.AddResult(Result{
		Timestamp:       now,
		Operation:       "GET",
		ObjectKey:       "key1.txt",
		TTFB:            50 * time.Millisecond,
		TTLB:            100 * time.Millisecond,
		BytesDownloaded: 1024,
		Error:           "",
	})
	stats.AddResult(Result{
		Timestamp:       now.Add(100 * time.Millisecond),
		Operation:       "GET",
		ObjectKey:       "key2.txt",
		TTFB:            30 * time.Millisecond,
		TTLB:            80 * time.Millisecond,
		BytesDownloaded: 2048,
		Error:           "",
	})
	stats.AddResult(Result{
		Timestamp:       now.Add(200 * time.Millisecond),
		Operation:       "GET",
		ObjectKey:       "key3.txt",
		TTFB:            100 * time.Millisecond,
		TTLB:            200 * time.Millisecond,
		BytesDownloaded: 4096,
		Error:           "",
	})

	// Failed GET operation
	stats.AddResult(Result{
		Timestamp:       now.Add(300 * time.Millisecond),
		Operation:       "GET",
		ObjectKey:       "key4.txt",
		TTFB:            -1,
		TTLB:            -1,
		BytesDownloaded: 0,
		Error:           "test error",
	})

	// Successful PUT operations
	stats.AddResult(Result{
		Timestamp:     now.Add(400 * time.Millisecond),
		Operation:     "PUT",
		ObjectKey:     "key5.txt",
		TTFB:          -1, // Not applicable for PUT
		TTLB:          150 * time.Millisecond,
		BytesUploaded: 2048,
		Error:         "",
	})
	stats.AddResult(Result{
		Timestamp:     now.Add(500 * time.Millisecond),
		Operation:     "PUT",
		ObjectKey:     "key6.txt",
		TTFB:          -1, // Not applicable for PUT
		TTLB:          250 * time.Millisecond,
		BytesUploaded: 4096,
		Error:         "",
	})

	// Failed PUT operation
	stats.AddResult(Result{
		Timestamp:     now.Add(600 * time.Millisecond),
		Operation:     "PUT",
		ObjectKey:     "key7.txt",
		TTFB:          -1,
		TTLB:          -1,
		BytesUploaded: 0,
		Error:         "test error",
	})

	// Calculate statistics
	endTime := now.Add(1 * time.Second)
	stats.Calculate(now, endTime)

	// Verify totals
	if stats.TotalRequests != 7 {
		t.Errorf("Expected TotalRequests=7, got %d", stats.TotalRequests)
	}
	if stats.TotalGets != 4 {
		t.Errorf("Expected TotalGets=4, got %d", stats.TotalGets)
	}
	if stats.TotalPuts != 3 {
		t.Errorf("Expected TotalPuts=3, got %d", stats.TotalPuts)
	}
	if stats.TotalErrors != 2 {
		t.Errorf("Expected TotalErrors=2, got %d", stats.TotalErrors)
	}
	if stats.TotalBytesDown != 7168 { // 1024 + 2048 + 4096
		t.Errorf("Expected TotalBytesDown=7168, got %d", stats.TotalBytesDown)
	}
	if stats.TotalBytesUp != 6144 { // 2048 + 4096
		t.Errorf("Expected TotalBytesUp=6144, got %d", stats.TotalBytesUp)
	}

	// Verify GET latency stats
	if stats.MinGetTTFB != 30*time.Millisecond {
		t.Errorf("Expected MinGetTTFB=30ms, got %v", stats.MinGetTTFB)
	}
	if stats.MaxGetTTFB != 100*time.Millisecond {
		t.Errorf("Expected MaxGetTTFB=100ms, got %v", stats.MaxGetTTFB)
	}
	if stats.MinGetTTLB != 80*time.Millisecond {
		t.Errorf("Expected MinGetTTLB=80ms, got %v", stats.MinGetTTLB)
	}
	if stats.MaxGetTTLB != 200*time.Millisecond {
		t.Errorf("Expected MaxGetTTLB=200ms, got %v", stats.MaxGetTTLB)
	}

	// Verify PUT latency stats
	if stats.MinPutTTLB != 150*time.Millisecond {
		t.Errorf("Expected MinPutTTLB=150ms, got %v", stats.MinPutTTLB)
	}
	if stats.MaxPutTTLB != 250*time.Millisecond {
		t.Errorf("Expected MaxPutTTLB=250ms, got %v", stats.MaxPutTTLB)
	}

	// Test averages (approximately)
	expectedAvgGetTTFB := (50 + 30 + 100) * time.Millisecond / 3
	if stats.AvgGetTTFB != expectedAvgGetTTFB {
		t.Errorf("Expected AvgGetTTFB=%v, got %v", expectedAvgGetTTFB, stats.AvgGetTTFB)
	}

	expectedAvgGetTTLB := (100 + 80 + 200) * time.Millisecond / 3
	if stats.AvgGetTTLB != expectedAvgGetTTLB {
		t.Errorf("Expected AvgGetTTLB=%v, got %v", expectedAvgGetTTLB, stats.AvgGetTTLB)
	}

	expectedAvgPutTTLB := (150 + 250) * time.Millisecond / 2
	if stats.AvgPutTTLB != expectedAvgPutTTLB {
		t.Errorf("Expected AvgPutTTLB=%v, got %v", expectedAvgPutTTLB, stats.AvgPutTTLB)
	}

	// Test percentiles
	// P50 for 3 values should be the middle value when sorted
	if stats.P50GetTTFB != 50*time.Millisecond {
		t.Errorf("Expected P50GetTTFB=50ms, got %v", stats.P50GetTTFB)
	}

	// P90 for 2 values should be the higher value
	if stats.P90PutTTLB != 250*time.Millisecond {
		t.Errorf("Expected P90PutTTLB=250ms, got %v", stats.P90PutTTLB)
	}
}

func TestPrintSummary(t *testing.T) {
	stats := NewStats()

	// Add some test data
	now := time.Now()
	stats.AddResult(Result{
		Timestamp:       now,
		Operation:       "GET",
		ObjectKey:       "key1.txt",
		TTFB:            50 * time.Millisecond,
		TTLB:            100 * time.Millisecond,
		BytesDownloaded: 1024,
		Error:           "",
	})
	stats.AddResult(Result{
		Timestamp:     now.Add(100 * time.Millisecond),
		Operation:     "PUT",
		ObjectKey:     "key2.txt",
		TTFB:          -1,
		TTLB:          150 * time.Millisecond,
		BytesUploaded: 2048,
		Error:         "",
	})

	// Calculate statistics
	endTime := now.Add(1 * time.Second)
	stats.Calculate(now, endTime)

	// Test PrintSummary output
	var buf bytes.Buffer
	stats.PrintSummary(&buf)

	output := buf.String()

	// Verify the summary contains expected sections
	if !strings.Contains(output, "--- Stress Test Summary ---") {
		t.Error("Summary output missing title section")
	}
	if !strings.Contains(output, "GET Operations") {
		t.Error("Summary output missing GET Operations section")
	}
	if !strings.Contains(output, "PUT Operations") {
		t.Error("Summary output missing PUT Operations section")
	}
	if !strings.Contains(output, "Total Requests: 2") {
		t.Error("Summary output missing or incorrect total requests count")
	}
}

func TestWriteResultsCSV(t *testing.T) {
	// Create test results
	now := time.Now()
	results := []Result{
		{
			Timestamp:       now,
			Operation:       "GET",
			ObjectKey:       "key1.txt",
			TTFB:            50 * time.Millisecond,
			TTLB:            100 * time.Millisecond,
			BytesDownloaded: 1024,
			Error:           "",
		},
		{
			Timestamp:     now.Add(100 * time.Millisecond),
			Operation:     "PUT",
			ObjectKey:     "key2.txt",
			TTFB:          -1,
			TTLB:          150 * time.Millisecond,
			BytesUploaded: 2048,
			Error:         "",
		},
		{
			Timestamp:       now.Add(200 * time.Millisecond),
			Operation:       "GET",
			ObjectKey:       "key3.txt",
			TTFB:            -1,
			TTLB:            -1,
			BytesDownloaded: 0,
			Error:           "test error",
		},
	}

	// Write CSV to temporary file
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "test_results.csv")

	err := WriteResultsCSV(results, csvPath)
	if err != nil {
		t.Fatalf("WriteResultsCSV failed: %v", err)
	}

	// Verify file exists and has content
	fileInfo, err := os.Stat(csvPath)
	if err != nil {
		t.Fatalf("Failed to stat CSV file: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Error("CSV file exists but is empty")
	}

	// Read file content
	content, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	// Check for expected header and data
	contentStr := string(content)
	if !strings.Contains(contentStr, "Timestamp,Operation,ObjectKey,TTFB(ms),TTLB(ms),BytesDownloaded,BytesUploaded,Error") {
		t.Error("CSV file missing expected header")
	}
	if !strings.Contains(contentStr, "GET,key1.txt") {
		t.Error("CSV file missing expected GET record")
	}
	if !strings.Contains(contentStr, "PUT,key2.txt") {
		t.Error("CSV file missing expected PUT record")
	}
	if !strings.Contains(contentStr, "test error") {
		t.Error("CSV file missing expected error record")
	}
}
