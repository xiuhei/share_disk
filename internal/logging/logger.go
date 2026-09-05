package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger with additional convenience methods.
type Logger struct {
	*slog.Logger
	file *os.File
}

// New creates a new Logger with the given configuration.
func New(level, format, output string) (*Logger, error) {
	// Parse log level
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Determine output writer
	var writer io.Writer
	var file *os.File
	switch strings.ToLower(output) {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		var err error
		file, err = os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		writer = file
	}

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	case "text":
		handler = slog.NewTextHandler(writer, opts)
	default:
		handler = slog.NewJSONHandler(writer, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
		file:   file,
	}, nil
}

// WithComponent returns a logger with a component field.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

// WithService returns a logger with a service field.
func (l *Logger) WithService(service string) *Logger {
	return &Logger{
		Logger: l.Logger.With("service", service),
	}
}

// WithAccount returns a logger with an account ID field (hashed for privacy).
func (l *Logger) WithAccount(accountIDHash string) *Logger {
	return &Logger{
		Logger: l.Logger.With("account_id_hash", accountIDHash),
	}
}

// WithDevice returns a logger with a device ID field.
func (l *Logger) WithDevice(deviceID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("device_id", deviceID),
	}
}

// WithTask returns a logger with a task ID field.
func (l *Logger) WithTask(taskID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("task_id", taskID),
	}
}

// WithError returns a logger with an error field.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.Logger.With("error", err),
	}
}

// WithRequestID returns a logger with a request ID field.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("request_id", requestID),
	}
}

// WithTraceID returns a logger with a trace ID field.
func (l *Logger) WithTraceID(traceID string) *Logger {
	return &Logger{
		Logger: l.Logger.With("trace_id", traceID),
	}
}

// Close closes the logger and any open file handles.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
