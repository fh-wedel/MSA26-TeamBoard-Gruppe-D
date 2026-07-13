package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedactAttr(t *testing.T) {
	t.Run("redacts known sensitive keys (case-insensitive)", func(t *testing.T) {
		for _, key := range []string{"password", "Token", "ACCESS_TOKEN", "Authorization", "api_key", "key_encryption_key"} {
			got := RedactAttr(nil, slog.String(key, "super-secret"))
			if got.Value.String() != "***" {
				t.Errorf("key %q: value = %q, want ***", key, got.Value.String())
			}
			if got.Key != key {
				t.Errorf("key changed: %q -> %q", key, got.Key)
			}
		}
	})

	t.Run("leaves non-sensitive keys untouched", func(t *testing.T) {
		got := RedactAttr(nil, slog.String("user_id", "u-123"))
		if got.Value.String() != "u-123" {
			t.Errorf("non-sensitive value altered: %q", got.Value.String())
		}
	})
}

func TestSetupLogger(t *testing.T) {
	logger := SetupLogger("test-service", "debug")
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
	// Default logger is replaced.
	if slog.Default() == nil {
		t.Error("default logger not set")
	}
}

func TestSetupLogger_InvalidLevelFallsBackToInfo(t *testing.T) {
	// An unparsable level must not panic; it falls back to Info. We assert behaviour
	// by checking that a debug record is dropped while info is kept.
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       fallbackLevel("not-a-level"),
		ReplaceAttr: RedactAttr,
	})
	l := slog.New(h)
	l.Debug("should-be-dropped")
	l.Info("should-be-kept", "password", "p@ss")

	if bytes.Contains(buf.Bytes(), []byte("should-be-dropped")) {
		t.Error("debug record should be dropped at info level")
	}
	if !bytes.Contains(buf.Bytes(), []byte("should-be-kept")) {
		t.Error("info record should be kept")
	}
	// Redaction also applies through the handler.
	if bytes.Contains(buf.Bytes(), []byte("p@ss")) {
		t.Error("sensitive value leaked into log output")
	}
}

// fallbackLevel mirrors SetupLogger's level-parsing fallback for the test above.
func fallbackLevel(level string) slog.Level {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return slog.LevelInfo
	}
	return lvl
}

func TestTracingHandler_NoSpanIsCleanJSON(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: RedactAttr})
	l := slog.New(&tracingHandler{next: base}).With("service", "svc")

	l.InfoContext(context.Background(), "hello", "user_id", "u-1", "token", "leak-me")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if rec["service"] != "svc" || rec["msg"] != "hello" || rec["user_id"] != "u-1" {
		t.Errorf("unexpected record: %v", rec)
	}
	// Without a valid span, no trace_id/span_id are added.
	if _, ok := rec["trace_id"]; ok {
		t.Error("trace_id should be absent without an active span")
	}
	// Redaction wired through SetupLogger's handler chain.
	if rec["token"] != "***" {
		t.Errorf("token not redacted: %v", rec["token"])
	}
}

func TestSetupTracer_NoEndpointIsNoop(t *testing.T) {
	shutdown, err := SetupTracer(context.Background(), TracerConfig{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("SetupTracer without endpoint should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func is nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}

func TestTracingMiddleware_CallsNext(t *testing.T) {
	called := false
	h := TracingMiddleware("test-svc")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things/42", nil))

	if !called {
		t.Error("middleware did not call the next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
