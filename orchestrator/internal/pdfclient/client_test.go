package pdfclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/model"
)

// TestRebuildStreamingBodyKeepAlive is a regression test: the request context
// must stay alive while the response body is being read, otherwise Rebuild
// fails with "context canceled" when the server streams the PDF slowly.
func TestRebuildStreamingBodyKeepAlive(t *testing.T) {
	const chunk = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // send headers immediately; the old code canceled here
		}
		time.Sleep(50 * time.Millisecond) // ensure the client is reading the body
		for i := 0; i < 8; i++ {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, time.Minute)
	out := filepath.Join(t.TempDir(), "out.pdf")
	if err := c.Rebuild(context.Background(), model.RebuildInput{Title: "Leaky ctx", Chapters: nil}, out); err != nil {
		t.Fatalf("Rebuild with slow streaming body: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) != 8*len(chunk) {
		t.Fatalf("unexpected output size: got %d, want %d", len(data), 8*len(chunk))
	}
}

// TestRebuildTimeout verifies a deadline still aborts a stalled body.
func TestRebuildTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	c := New(srv.URL, 100*time.Millisecond)
	out := filepath.Join(t.TempDir(), "out.pdf")
	err := c.Rebuild(context.Background(), model.RebuildInput{Title: "t", Chapters: nil}, out)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
