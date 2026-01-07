package croupier

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewDefaultLogger(t *testing.T) {
	t.Parallel()

	logger := NewDefaultLogger(true, nil)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.debug != true {
		t.Error("expected debug to be true")
	}
}

func TestNewDefaultLoggerWithNilWriter(t *testing.T) {
	t.Parallel()

	logger := NewDefaultLogger(false, nil)
	if logger.out == nil {
		t.Error("expected default stdout writer")
	}
}

func TestNewDefaultLoggerWithCustomWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewDefaultLogger(true, &buf)

	logger.Infof("test message")
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("expected log message in output, got: %s", buf.String())
	}
}

func TestDefaultLogger_Debugf(t *testing.T) {
	t.Parallel()

	t.Run("writes when debug is true", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := NewDefaultLogger(true, &buf)

		logger.Debugf("debug %s", "message")
		output := buf.String()

		if !strings.Contains(output, "[DEBUG]") {
			t.Error("expected [DEBUG] prefix")
		}
		if !strings.Contains(output, "debug message") {
			t.Error("expected debug message")
		}
	})

	t.Run("does not write when debug is false", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		logger := NewDefaultLogger(false, &buf)

		logger.Debugf("debug %s", "message")
		output := buf.String()

		if output != "" {
			t.Errorf("expected empty output, got: %s", output)
		}
	})
}

func TestDefaultLogger_Infof(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewDefaultLogger(true, &buf)

	logger.Infof("info %s", "message")
	output := buf.String()

	if !strings.Contains(output, "[INFO]") {
		t.Error("expected [INFO] prefix")
	}
	if !strings.Contains(output, "info message") {
		t.Error("expected info message")
	}
}

func TestDefaultLogger_Warnf(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewDefaultLogger(true, &buf)

	logger.Warnf("warning %s", "message")
	output := buf.String()

	if !strings.Contains(output, "[WARN]") {
		t.Error("expected [WARN] prefix")
	}
	if !strings.Contains(output, "warning message") {
		t.Error("expected warning message")
	}
}

func TestDefaultLogger_Errorf(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewDefaultLogger(true, &buf)

	logger.Errorf("error %s", "message")
	output := buf.String()

	if !strings.Contains(output, "[ERROR]") {
		t.Error("expected [ERROR] prefix")
	}
	if !strings.Contains(output, "error message") {
		t.Error("expected error message")
	}
}

func TestNoOpLogger(t *testing.T) {
	t.Parallel()

	var logger NoOpLogger

	// These should not panic
	logger.Debugf("debug")
	logger.Infof("info")
	logger.Warnf("warn")
	logger.Errorf("error")
}

func TestSetGlobalLogger(t *testing.T) {
	t.Parallel()

	// Save original logger
	original := GetGlobalLogger()
	defer SetGlobalLogger(original)

	customLogger := &NoOpLogger{}
	SetGlobalLogger(customLogger)

	retrieved := GetGlobalLogger()
	if retrieved != customLogger {
		t.Error("expected custom logger to be returned")
	}
}

func TestGetGlobalLogger(t *testing.T) {
	t.Parallel()

	logger := GetGlobalLogger()
	if logger == nil {
		t.Error("expected non-nil global logger")
	}
}

func TestLogHelpers(t *testing.T) {
	t.Parallel()

	t.Run("logDebugf", func(t *testing.T) {
		t.Parallel()

		// Should not panic
		logDebugf("test %s", "debug")
	})

	t.Run("logInfof", func(t *testing.T) {
		t.Parallel()

		// Should not panic
		logInfof("test %s", "info")
	})
}
