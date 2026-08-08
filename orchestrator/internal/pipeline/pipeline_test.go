package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vribeiro19/books-translator/api/orchestrator/internal/model"
	"github.com/vribeiro19/books-translator/api/orchestrator/internal/store"
)

var errExtract = errors.New("extraction failed: boom")

type fakeTranslator struct {
	delay time.Duration
}

func (f fakeTranslator) TranslateChapter(ctx context.Context, chapter model.Chapter) ([]model.Block, error) {
	if f.delay > 0 {
		timer := time.NewTimer(f.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	out := make([]model.Block, 0, len(chapter.Blocks))
	for _, b := range chapter.Blocks {
		out = append(out, model.Block{Type: b.Type, Level: b.Level, Text: "PT: " + b.Text})
	}
	if len(out) == 0 {
		out = append(out, model.Block{Type: "paragraph", Level: 0, Text: "PT: vazio"})
	}
	return out, nil
}

type fakePDFService struct {
	result     *model.ExtractResult
	extractErr error
}

func (f *fakePDFService) Extract(_ context.Context, _ string) (*model.ExtractResult, error) {
	if f.extractErr != nil {
		return nil, f.extractErr
	}
	return f.result, nil
}

func (f *fakePDFService) Rebuild(_ context.Context, input model.RebuildInput, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("PDF:" + input.Title + "\n")
	for _, ch := range input.Chapters {
		sb.WriteString("#" + ch.Title + "\n")
		for _, b := range ch.Blocks {
			sb.WriteString(b.Text + "\n")
		}
	}
	return os.WriteFile(outPath, []byte(sb.String()), 0o644)
}

func testBook() *model.ExtractResult {
	return &model.ExtractResult{
		PageCount: 4,
		Chapters: []model.Chapter{
			{Title: "Intro", Blocks: []model.Block{
				{Type: "paragraph", Level: 0, Text: "hello world"},
			}},
			{Title: "Methods", Blocks: []model.Block{
				{Type: "heading", Level: 1, Text: "Sampling"},
				{Type: "paragraph", Level: 0, Text: "we sample lots"},
			}},
		},
	}
}

func newTestPipeline(t *testing.T, pdf PDFService) (*Pipeline, *store.Store) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return New(st, pdf, fakeTranslator{}, 2, logger), st
}

func waitStatus(t *testing.T, st *store.Store, id string, status store.JobStatus) *store.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := st.Get(id)
		if err == nil && job.Status == status {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, _ := st.Get(id)
	t.Fatalf("timed out waiting for status %s; last: %+v", status, job)
	return nil
}

func TestRunCompletesJob(t *testing.T) {
	p, st := newTestPipeline(t, &fakePDFService{result: testBook()})

	job, err := st.Create("j1", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p.Start(job.ID)

	done := waitStatus(t, st, "j1", store.StatusCompleted)
	if done.ChaptersDone != 2 || done.ChaptersTotal != 2 {
		t.Fatalf("unexpected progress: %d/%d", done.ChaptersDone, done.ChaptersTotal)
	}
	if _, err := os.Stat(st.ResultPath("j1")); err != nil {
		t.Fatalf("result pdf missing: %v", err)
	}
}

func TestRunPreviewFlipsPreviewReady(t *testing.T) {
	pdf := &fakePDFService{result: testBook()}
	st, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	p := New(st, pdf, fakeTranslator{delay: 150 * time.Millisecond}, 2, logger)

	job, err := st.Create("j2", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p.Start(job.ID)

	preview := waitStatus(t, st, "j2", store.StatusPreviewReady)
	if preview.ChaptersDone < 1 {
		t.Fatalf("expected at least chapter 0 done, got %d", preview.ChaptersDone)
	}
	if _, err := os.Stat(st.PreviewPath("j2")); err != nil {
		t.Fatalf("preview pdf missing: %v", err)
	}

	done := waitStatus(t, st, "j2", store.StatusCompleted)
	if done.ChaptersDone != 2 {
		t.Fatalf("expected 2 chapters done, got %d", done.ChaptersDone)
	}
	if _, err := os.Stat(st.ResultPath("j2")); err != nil {
		t.Fatalf("result pdf missing: %v", err)
	}
}

func TestPipelineExtractFailureFailsJob(t *testing.T) {
	p, st := newTestPipeline(t, &fakePDFService{extractErr: errExtract})
	job, err := st.Create("j3", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p.Start(job.ID)

	failed := waitStatus(t, st, "j3", store.StatusFailed)
	if !strings.Contains(failed.Error, "extraction failed") {
		t.Fatalf("expected extraction error, got %q", failed.Error)
	}
}
