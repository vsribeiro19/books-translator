package store

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	job, err := s.Create("j1", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if job.Status != StatusPending {
		t.Fatalf("expected pending, got %s", job.Status)
	}
	if !job.PreviewRequested {
		t.Fatal("expected preview requested")
	}

	got, err := s.Get("j1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "j1" {
		t.Fatalf("expected id j1, got %s", got.ID)
	}

	if _, err := s.Get("nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSetStatusTransitions(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create("j1", false); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetStatus("j1", StatusProcessing, ""); err != nil {
		t.Fatalf("SetStatus processing: %v", err)
	}
	if err := s.SetStatus("j1", StatusCompleted, ""); err != nil {
		t.Fatalf("SetStatus completed: %v", err)
	}
	// Terminal states reject further transitions.
	if err := s.SetStatus("j1", StatusFailed, "boom"); err != ErrDone {
		t.Fatalf("expected ErrDone, got %v", err)
	}

	got, _ := s.Get("j1")
	if got.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestRecordChapterProgress(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create("j1", false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetInfo("j1", "Book", 120, 3); err != nil {
		t.Fatalf("SetInfo: %v", err)
	}
	blocks := []model.Block{{Type: "paragraph", Level: 0, Text: "hi"}}
	if err := s.RecordChapter("j1", 0, blocks); err != nil {
		t.Fatalf("RecordChapter: %v", err)
	}
	if err := s.RecordChapterError("j1", 2); err != nil {
		t.Fatalf("RecordChapterError: %v", err)
	}

	job, _ := s.Get("j1")
	if job.ChaptersDone != 1 || job.ChaptersTotal != 3 {
		t.Fatalf("expected progress 1/3, got %d/%d", job.ChaptersDone, job.ChaptersTotal)
	}
	if got := job.ChapterResults[0]; len(got) != 1 || got[0].Text != "hi" {
		t.Fatalf("unexpected chapter results: %+v", job.ChapterResults)
	}
	if len(job.FailedChapters) != 1 || job.FailedChapters[0] != 2 {
		t.Fatalf("unexpected failed chapters: %v", job.FailedChapters)
	}
}

func TestRecoverFailsInterruptedJobs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Create("done", false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = s.SetStatus("done", StatusCompleted, "")

	if _, err := s.Create("running", false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = s.SetStatus("running", StatusProcessing, "")

	// New store over the same dir simulates a restart.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New2: %v", err)
	}
	if err := s2.Recover(); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	done, _ := s2.Get("done")
	if done.Status != StatusCompleted {
		t.Fatalf("done should stay completed, got %s", done.Status)
	}
	running, err := s2.Get("running")
	if err != nil {
		t.Fatalf("Get running: %v", err)
	}
	if running.Status != StatusFailed {
		t.Fatalf("running should be failed after recover, got %s", running.Status)
	}
	if running.Error == "" {
		t.Fatal("expected error message after recover")
	}
}

func TestConcurrentRecordChapter(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Create("j1", false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetInfo("j1", "Book", 10, 20); err != nil {
		t.Fatalf("SetInfo: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.RecordChapter("j1", i, []model.Block{{Type: "paragraph", Level: 0, Text: "x"}})
		}(i)
	}
	wg.Wait()

	job, _ := s.Get("j1")
	if job.ChaptersDone != 20 {
		t.Fatalf("expected 20 chapters done, got %d", job.ChaptersDone)
	}
}

func TestPaths(t *testing.T) {
	s := newTestStore(t)
	if got := s.OriginalPath("j1"); filepath.Base(got) != "original.pdf" {
		t.Fatalf("unexpected original path: %s", got)
	}
	if got := s.PreviewPath("j1"); filepath.Base(got) != "preview.pdf" {
		t.Fatalf("unexpected preview path: %s", got)
	}
	if got := s.ResultPath("j1"); filepath.Base(got) != "result.pdf" {
		t.Fatalf("unexpected result path: %s", got)
	}
}
