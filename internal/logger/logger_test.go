package logger_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gids/internal/logger"
)

func TestNew_InfoLevel_SuppressesDebug(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, false)
	l.Debug("secret debug message")
	if strings.Contains(buf.String(), "secret debug message") {
		t.Error("debug message should be suppressed at info level")
	}
}

func TestNew_VerboseLevel_ShowsDebug(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, true)
	l.Debug("visible debug message")
	if !strings.Contains(buf.String(), "visible debug message") {
		t.Error("debug message should appear at debug level")
	}
}

func TestFromContext_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(&buf, true)
	ctx := logger.WithContext(context.Background(), l)
	got := logger.FromContext(ctx)
	got.Debug("roundtrip message")
	if !strings.Contains(buf.String(), "roundtrip message") {
		t.Error("logger retrieved from context should write to the original buffer")
	}
}

func TestFromContext_FallsBackToDefault(t *testing.T) {
	got := logger.FromContext(context.Background())
	if got == nil {
		t.Error("FromContext should never return nil")
	}
}
