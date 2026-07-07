package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewJSONLogger(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		action   string
		wantErr  bool
	}{
		{
			name:     "valid msgraphtool logger",
			toolName: "msgraphtool",
			action:   "sendmail",
			wantErr:  false,
		},
		{
			name:     "valid smtptool logger",
			toolName: "smtptool",
			action:   "testauth",
			wantErr:  false,
		},
		{
			name:     "empty toolname",
			toolName: "",
			action:   "test",
			wantErr:  false, // Should still work
		},
		{
			name:     "empty action",
			toolName: "testtool",
			action:   "",
			wantErr:  false, // Should still work
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewJSONLogger(tt.toolName, tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewJSONLogger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if logger == nil {
					t.Fatal("NewJSONLogger() returned nil logger with no error")
				}
				defer func() { _ = logger.Close() }()
				defer func() { _ = os.Remove(logger.file.Name()) }()

				// Verify file was created
				if _, err := os.Stat(logger.file.Name()); os.IsNotExist(err) {
					t.Errorf("Log file was not created: %s", logger.file.Name())
				}

				// Verify filename format
				expectedSuffix := ".jsonl"
				if !strings.HasSuffix(logger.file.Name(), expectedSuffix) {
					t.Errorf("Log file should end with .jsonl, got: %s", logger.file.Name())
				}
			}
		})
	}
}

func TestJSONLogger_WriteHeaderAndRow(t *testing.T) {
	logger, err := NewJSONLogger("testool", "testaction")
	if err != nil {
		t.Fatalf("NewJSONLogger() error = %v", err)
	}
	defer func() { _ = logger.Close() }()
	defer func() { _ = os.Remove(logger.file.Name()) }()

	// Write header
	columns := []string{"Action", "Status", "Server", "Port"}
	if err := logger.WriteHeader(columns); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}

	// Write a row
	row := []string{"testconnect", "SUCCESS", "smtp.example.com", "587"}
	if err := logger.WriteRow(row); err != nil {
		t.Fatalf("WriteRow() error = %v", err)
	}

	// Force close to flush
	_ = logger.Close()

	// Read and verify the file
	data, err := os.ReadFile(logger.file.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 JSON line, got %d", len(lines))
	}

	// Parse JSON and verify structure
	var jsonObj map[string]string
	if err := json.Unmarshal([]byte(lines[0]), &jsonObj); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify timestamp exists
	if _, ok := jsonObj["timestamp"]; !ok {
		t.Error("JSON object missing 'timestamp' field")
	}

	// Verify all columns are present with correct values
	expectedFields := map[string]string{
		"Action": "testconnect",
		"Status": "SUCCESS",
		"Server": "smtp.example.com",
		"Port":   "587",
	}

	for key, expectedValue := range expectedFields {
		if actualValue, ok := jsonObj[key]; !ok {
			t.Errorf("JSON object missing field '%s'", key)
		} else if actualValue != expectedValue {
			t.Errorf("Field '%s' = %q, want %q", key, actualValue, expectedValue)
		}
	}
}

func TestJSONLogger_MultipleRows(t *testing.T) {
	// Clean up any existing file first
	tempDir := os.TempDir()
	dateStr := time.Now().Format("2006-01-02")
	testFile := filepath.Join(tempDir, "_testtool_testaction_"+dateStr+".jsonl")
	_ = os.Remove(testFile)

	logger, err := NewJSONLogger("testtool", "testaction")
	if err != nil {
		t.Fatalf("NewJSONLogger() error = %v", err)
	}
	defer func() { _ = logger.Close() }()
	defer func() { _ = os.Remove(logger.file.Name()) }()

	// Write header
	columns := []string{"ID", "Status"}
	if err := logger.WriteHeader(columns); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}

	// Write multiple rows
	rows := [][]string{
		{"1", "SUCCESS"},
		{"2", "FAILURE"},
		{"3", "SUCCESS"},
	}

	for _, row := range rows {
		if err := logger.WriteRow(row); err != nil {
			t.Fatalf("WriteRow() error = %v", err)
		}
	}

	_ = logger.Close()

	// Read and verify
	data, err := os.ReadFile(logger.file.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 JSON lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var jsonObj map[string]string
		if err := json.Unmarshal([]byte(line), &jsonObj); err != nil {
			t.Errorf("Line %d: Failed to parse JSON: %v", i+1, err)
		}
	}
}

func TestJSONLogger_ErrorCases(t *testing.T) {
	t.Run("WriteRow before WriteHeader", func(t *testing.T) {
		logger, err := NewJSONLogger("testtool", "testaction")
		if err != nil {
			t.Fatalf("NewJSONLogger() error = %v", err)
		}
		defer func() { _ = logger.Close() }()
		defer func() { _ = os.Remove(logger.file.Name()) }()

		// Try to write row without header
		row := []string{"value1", "value2"}
		err = logger.WriteRow(row)
		if err == nil {
			t.Error("WriteRow() should error when called before WriteHeader")
		}
	})

	t.Run("Row length mismatch", func(t *testing.T) {
		logger, err := NewJSONLogger("testtool", "testaction")
		if err != nil {
			t.Fatalf("NewJSONLogger() error = %v", err)
		}
		defer func() { _ = logger.Close() }()
		defer func() { _ = os.Remove(logger.file.Name()) }()

		// Write header with 2 columns
		if err := logger.WriteHeader([]string{"Col1", "Col2"}); err != nil {
			t.Fatalf("WriteHeader() error = %v", err)
		}

		// Try to write row with 3 values
		row := []string{"val1", "val2", "val3"}
		err = logger.WriteRow(row)
		if err == nil {
			t.Error("WriteRow() should error when row length doesn't match header")
		}
	})
}

func TestJSONLogger_Append(t *testing.T) {
	tempDir := os.TempDir()
	dateStr := time.Now().Format("2006-01-02")
	fileName := filepath.Join(tempDir, "_testtool_testaction_"+dateStr+".jsonl")

	// Clean up any existing file
	_ = os.Remove(fileName)

	// Create first logger and write data
	logger1, err := NewJSONLogger("testtool", "testaction")
	if err != nil {
		t.Fatalf("NewJSONLogger() error = %v", err)
	}
	defer func() { _ = os.Remove(logger1.file.Name()) }()

	_ = logger1.WriteHeader([]string{"ID"})
	_ = logger1.WriteRow([]string{"1"})
	_ = logger1.Close()

	// Create second logger (should append)
	logger2, err := NewJSONLogger("testtool", "testaction")
	if err != nil {
		t.Fatalf("NewJSONLogger() error = %v", err)
	}
	defer func() { _ = logger2.Close() }()

	_ = logger2.WriteHeader([]string{"ID"})
	_ = logger2.WriteRow([]string{"2"})
	_ = logger2.Close()

	// Verify both rows exist
	data, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 JSON lines after append, got %d", len(lines))
	}
}

func TestJSONLogger_ShouldWriteHeader(t *testing.T) {
	t.Run("new file", func(t *testing.T) {
		logger, err := NewJSONLogger("testtool", "testaction")
		if err != nil {
			t.Fatalf("NewJSONLogger() error = %v", err)
		}
		defer func() { _ = logger.Close() }()
		defer func() { _ = os.Remove(logger.file.Name()) }()

		shouldWrite, err := logger.ShouldWriteHeader()
		if err != nil {
			t.Fatalf("ShouldWriteHeader() error = %v", err)
		}
		if !shouldWrite {
			t.Error("ShouldWriteHeader() should return true for new file")
		}
	})

	t.Run("existing file with data", func(t *testing.T) {
		logger, err := NewJSONLogger("testtool", "testaction")
		if err != nil {
			t.Fatalf("NewJSONLogger() error = %v", err)
		}
		defer func() { _ = os.Remove(logger.file.Name()) }()

		_ = logger.WriteHeader([]string{"ID"})
		_ = logger.WriteRow([]string{"1"})
		_ = logger.Close()

		// Reopen
		logger2, err := NewJSONLogger("testtool", "testaction")
		if err != nil {
			t.Fatalf("NewJSONLogger() error = %v", err)
		}
		defer func() { _ = logger2.Close() }()

		shouldWrite, err := logger2.ShouldWriteHeader()
		if err != nil {
			t.Fatalf("ShouldWriteHeader() error = %v", err)
		}
		if shouldWrite {
			t.Error("ShouldWriteHeader() should return false for existing file with data")
		}
	})
}

func TestJSONLogger_PeriodicFlushing(t *testing.T) {
	logger, err := NewJSONLogger("testtool", "testaction")
	if err != nil {
		t.Fatalf("NewJSONLogger() error = %v", err)
	}
	defer func() { _ = logger.Close() }()
	defer func() { _ = os.Remove(logger.file.Name()) }()

	// Write header
	_ = logger.WriteHeader([]string{"ID"})

	// Write rows without closing to test periodic flushing
	for i := 0; i < 15; i++ {
		if err := logger.WriteRow([]string{string(rune('0' + i))}); err != nil {
			t.Fatalf("WriteRow() error = %v", err)
		}
	}

	// Give time for flush (flushEvery = 10)
	time.Sleep(100 * time.Millisecond)

	// Try to read file (should have at least first 10 rows)
	file, err := os.Open(logger.file.Name())
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}

	if lineCount < 10 {
		t.Errorf("Expected at least 10 flushed lines, got %d", lineCount)
	}
}
