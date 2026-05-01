package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// readingHandler reads the entire request body. Returns 200 on
// successful read, surfaces body-size errors as 413 / generic errors
// as 500.
func readingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			if IsBodyTooLarge(err) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func TestMaxBodySize_AllowsUnderLimit(t *testing.T) {
	mw := MaxBodySize(1024)
	body := strings.NewReader(strings.Repeat("a", 512))
	req := httptest.NewRequest(http.MethodPost, "/x", body)
	req.ContentLength = 512
	rec := httptest.NewRecorder()

	mw(readingHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMaxBodySize_BlocksOverLimit(t *testing.T) {
	mw := MaxBodySize(1024)
	body := strings.NewReader(strings.Repeat("a", 2048))
	req := httptest.NewRequest(http.MethodPost, "/x", body)
	req.ContentLength = 2048
	rec := httptest.NewRecorder()

	mw(readingHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestMaxBodySize_BoundaryAtExactLimit(t *testing.T) {
	// Body of exactly `limit` bytes is allowed — MaxBytesReader trips
	// only when the reader would have to return one more byte.
	mw := MaxBodySize(1024)
	body := strings.NewReader(strings.Repeat("a", 1024))
	req := httptest.NewRequest(http.MethodPost, "/x", body)
	req.ContentLength = 1024
	rec := httptest.NewRecorder()

	mw(readingHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 at exact-limit body size", rec.Code)
	}
}

func TestIsBodyTooLarge_RecognizesMaxBytesError(t *testing.T) {
	if !IsBodyTooLarge(&http.MaxBytesError{Limit: 100}) {
		t.Error("expected true for *http.MaxBytesError")
	}
	if IsBodyTooLarge(io.EOF) {
		t.Error("expected false for io.EOF")
	}
	if IsBodyTooLarge(nil) {
		t.Error("expected false for nil")
	}
	if IsBodyTooLarge(errors.New("some other error")) {
		t.Error("expected false for unrelated error")
	}
}
