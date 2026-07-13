package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── json helpers ────────────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"name": "alice"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	var body struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Data["name"] != "alice" {
		t.Errorf("data not wrapped correctly: %v", body.Data)
	}
}

func TestWriteList(t *testing.T) {
	t.Run("with pagination", func(t *testing.T) {
		rec := httptest.NewRecorder()
		next := "abc"
		WriteList(rec, http.StatusOK, []int{1, 2}, &Pagination{NextCursor: &next, Limit: 50})

		var body struct {
			Data       []int       `json:"data"`
			Pagination *Pagination `json:"pagination"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.Data) != 2 || body.Pagination == nil || body.Pagination.Limit != 50 {
			t.Errorf("unexpected list response: %s", rec.Body.String())
		}
		if body.Pagination.NextCursor == nil || *body.Pagination.NextCursor != "abc" {
			t.Errorf("next_cursor not preserved")
		}
	})

	t.Run("omits pagination when nil", func(t *testing.T) {
		rec := httptest.NewRecorder()
		WriteList(rec, http.StatusOK, []int{}, nil)
		if got := rec.Body.String(); contains(got, "pagination") {
			t.Errorf("pagination should be omitted, got %s", got)
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ─── problem details ─────────────────────────────────────────────────────────

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	WriteProblem(rec, req, http.StatusNotFound, "thing not found")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q", ct)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Status != 404 || p.Detail != "thing not found" {
		t.Errorf("unexpected problem: %+v", p)
	}
	if p.Type != "https://teamboard.example/errors/not_found" {
		t.Errorf("type slug wrong: %s", p.Type)
	}
	if p.Title != http.StatusText(http.StatusNotFound) {
		t.Errorf("title = %q", p.Title)
	}
}

func TestStatusSlug(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:          "bad_request",
		http.StatusUnauthorized:        "unauthorized",
		http.StatusForbidden:           "forbidden",
		http.StatusNotFound:            "not_found",
		http.StatusConflict:            "conflict",
		http.StatusTooManyRequests:     "rate_limited",
		http.StatusInternalServerError: "internal_error",
		http.StatusTeapot:              "error", // default
	}
	for status, want := range cases {
		if got := statusSlug(status); got != want {
			t.Errorf("statusSlug(%d) = %q, want %q", status, got, want)
		}
	}
}

// ─── error mapping ───────────────────────────────────────────────────────────

type testDomainErr struct{ code string }

func (e testDomainErr) Error() string   { return e.code + " happened" }
func (e testDomainErr) GetCode() string { return e.code }

func TestMapErrorToHTTPStatus(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"task_not_found", http.StatusNotFound},
		{"permission_denied", http.StatusForbidden},
		{"validation_failed", http.StatusBadRequest},
		{"invalid_board_type", http.StatusBadRequest},
		{"password_too_weak", http.StatusBadRequest},
		{"already_member", http.StatusConflict},
		{"email_taken", http.StatusConflict},
		{"last_owner_protected", http.StatusConflict},
		{"rate_limited", http.StatusTooManyRequests},
		{"unauthorized", http.StatusUnauthorized},
		{"invalid_credentials", http.StatusUnauthorized},
		{"token_revoked", http.StatusUnauthorized},
		{"something_unmapped", http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := MapErrorToHTTPStatus(testDomainErr{c.code}); got != c.want {
			t.Errorf("MapErrorToHTTPStatus(%q) = %d, want %d", c.code, got, c.want)
		}
	}

	t.Run("non-domain error maps to 500", func(t *testing.T) {
		if got := MapErrorToHTTPStatus(errPlain("boom")); got != http.StatusInternalServerError {
			t.Errorf("got %d, want 500", got)
		}
	})
}

type errPlain string

func (e errPlain) Error() string { return string(e) }

func TestWriteError(t *testing.T) {
	t.Run("domain error is mapped", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		WriteError(rec, req, testDomainErr{"board_not_found"})

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
		var p Problem
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		if p.Type != "https://teamboard.example/errors/board_not_found" {
			t.Errorf("type = %s", p.Type)
		}
		if p.Title != "Board not found" {
			t.Errorf("humanized title = %q", p.Title)
		}
	})

	t.Run("non-domain error becomes 500", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		WriteError(rec, req, errPlain("kaboom"))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}

// ─── cursor pagination ───────────────────────────────────────────────────────

func TestCursorRoundTrip(t *testing.T) {
	type cur struct {
		ID   string `json:"id"`
		Page int    `json:"page"`
	}
	in := cur{ID: "x-1", Page: 3}
	enc, err := EncodeCursor(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out cur
	if err := DecodeCursor(enc, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != in {
		t.Errorf("round-trip mismatch: got %+v want %+v", out, in)
	}
}

func TestDecodeCursor_EmptyIsNoop(t *testing.T) {
	var out map[string]any
	if err := DecodeCursor("", &out); err != nil {
		t.Errorf("empty cursor should be a no-op, got %v", err)
	}
	if out != nil {
		t.Errorf("target should remain untouched, got %v", out)
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	var out map[string]any
	if err := DecodeCursor("!!!not-base64!!!", &out); err == nil {
		t.Error("expected error for invalid cursor")
	}
}
