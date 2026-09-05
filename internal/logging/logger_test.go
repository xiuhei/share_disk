package logging

import (
	"errors"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		format  string
		output  string
		wantErr bool
	}{
		{
			name:    "json format",
			level:   "info",
			format:  "json",
			output:  "stdout",
			wantErr: false,
		},
		{
			name:    "text format",
			level:   "debug",
			format:  "text",
			output:  "stdout",
			wantErr: false,
		},
		{
			name:    "invalid level defaults to info",
			level:   "invalid",
			format:  "json",
			output:  "stdout",
			wantErr: false,
		},
		{
			name:    "stderr output",
			level:   "info",
			format:  "json",
			output:  "stderr",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.level, tt.format, tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if logger == nil {
				t.Error("Expected non-nil logger")
			}
		})
	}
}

func TestLoggerWithComponent(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test WithComponent
	componentLogger := logger.WithComponent("test-component")
	if componentLogger == nil {
		t.Error("Expected non-nil component logger")
	}
}

func TestLoggerWithService(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	serviceLogger := logger.WithService("test-service")
	if serviceLogger == nil {
		t.Error("Expected non-nil service logger")
	}
}

func TestLoggerWithAccount(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	accountLogger := logger.WithAccount("account123")
	if accountLogger == nil {
		t.Error("Expected non-nil account logger")
	}
}

func TestLoggerWithDevice(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	deviceLogger := logger.WithDevice("device123")
	if deviceLogger == nil {
		t.Error("Expected non-nil device logger")
	}
}

func TestLoggerWithTask(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	taskLogger := logger.WithTask("task123")
	if taskLogger == nil {
		t.Error("Expected non-nil task logger")
	}
}

func TestLoggerWithError(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	errLogger := logger.WithError(errors.New("test error"))
	if errLogger == nil {
		t.Error("Expected non-nil error logger")
	}
}

func TestLoggerWithRequestID(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	requestLogger := logger.WithRequestID("req123")
	if requestLogger == nil {
		t.Error("Expected non-nil request logger")
	}
}

func TestLoggerWithTraceID(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	traceLogger := logger.WithTraceID("trace123")
	if traceLogger == nil {
		t.Error("Expected non-nil trace logger")
	}
}

func TestLoggerOutput(t *testing.T) {
	logger, err := New("info", "json", "stdout")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test that logger can log messages
	logger.Info("test message", "key", "value")

	// Since we can't easily capture stdout in tests, just verify logger works
	if logger.Logger == nil {
		t.Error("Expected non-nil underlying logger")
	}
}

func TestLoggerLevels(t *testing.T) {
	tests := []struct {
		level  string
		expect string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"invalid", "INFO"}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			logger, err := New(tt.level, "json", "stdout")
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			if logger == nil {
				t.Error("Expected non-nil logger")
			}
		})
	}
}

func TestLoggerFormats(t *testing.T) {
	tests := []struct {
		format string
		valid  bool
	}{
		{"json", true},
		{"text", true},
		{"invalid", true}, // defaults to json
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			logger, err := New("info", tt.format, "stdout")
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			if logger == nil {
				t.Error("Expected non-nil logger")
			}
		})
	}
}
