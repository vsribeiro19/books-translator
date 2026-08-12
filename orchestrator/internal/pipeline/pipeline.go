package pipeline

import (
	"context"
	"log/slog"
	"sync"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/model"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/store"
)

// ChapterTranslator translates a whole chapter preserving block structure.
type ChapterTranslator interface {
	TranslateChapter(ctx context.Context, chapter model.Chapter) ([]model.Block, error)
}

// PDFService abstracts the pdf-service HTTP client.
type PDFService interface {
	Extract(ctx context.Context, pdfPath string) (*model.ExtractResult, error)
	Rebuild(ctx context.Context, input model.RebuildInput, outPath string) error
}

// Pipeline runs the async job lifecycle: extract, translate per chapter
// (optionally previewing the first chapter), rebuild, and status updates.
type Pipeline struct {
	store       *store.Store
	pdf         PDFService
	translator  ChapterTranslator
	concurrency int
	logger      *slog.Logger
}

// New creates a Pipeline.
func New(st *store.Store, pdf PDFService, tr ChapterTranslator, concurrency int, logger *slog.Logger) *Pipeline {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Pipeline{
		store:       st,
		pdf:         pdf,
		translator:  tr,
		concurrency: concurrency,
		logger:      logger,
	}
}

type chapterTask struct {
	index   int
	chapter model.Chapter
}

// Start launches the job processing asynchronously.
func (p *Pipeline) Start(jobID string) {
	go p.run(jobID)
}

func (p *Pipeline) run(jobID string) {
	ctx := context.Background()
	logger := p.logger.With("job", jobID)

	fail := func(msg string) {
		if err := p.store.SetStatus(jobID, store.StatusFailed, msg); err != nil {
			logger.Error("fail job", "err", err)
		}
	}

	if err := p.store.SetStatus(jobID, store.StatusProcessing, ""); err != nil {
		logger.Warn("skip job (cannot start processing)", "err", err)
		return
	}

	// 1. Extract structured text from the uploaded PDF.
	result, err := p.pdf.Extract(ctx, p.store.OriginalPath(jobID))
	if err != nil {
		logger.Error("extract failed", "err", err)
		fail("extraction failed: " + err.Error())
		return
	}
	if len(result.Chapters) == 0 {
		// Degenerate document: treat the whole body as a single chapter.
		result.Chapters = []model.Chapter{{Title: "", Blocks: nil}}
	}

	title := result.Chapters[0].Title
	if err := p.store.SetInfo(jobID, title, result.PageCount, len(result.Chapters)); err != nil {
		logger.Error("set info", "err", err)
		fail("failed to record job info")
		return
	}
	logger.Info("extracted", "chapters", len(result.Chapters), "pages", result.PageCount)

	allOK := true
	next := 0
	if p.previewRequested(jobID) {
		// 2. Translate first chapter eagerly and ship a partial PDF.
		logger.Info("translating preview chapter")
		blocks, err := p.translator.TranslateChapter(ctx, result.Chapters[0])
		if err != nil {
			logger.Error("preview chapter failed", "err", err)
			_ = p.store.RecordChapterError(jobID, 0)
			allOK = false
		} else {
			if err := p.store.RecordChapter(jobID, 0, blocks); err != nil {
				logger.Error("record preview", "err", err)
			}
			p.publishPreview(jobID, title, result.Chapters[0].Title, blocks)
		}
		next = 1
	}

	// 3. Translate the remaining chapters concurrently.
	if next < len(result.Chapters) {
		tasks := make([]chapterTask, 0, len(result.Chapters)-next)
		for i := next; i < len(result.Chapters); i++ {
			tasks = append(tasks, chapterTask{index: i, chapter: result.Chapters[i]})
		}
		if ok := p.processChapters(ctx, jobID, tasks); !ok {
			allOK = false
		}
	}

	if !allOK {
		fail("one or more chapters failed after retries")
		return
	}

	// 4. Rebuild the full translated book.
	if err := p.buildResult(jobID, result.Chapters); err != nil {
		logger.Error("rebuild final", "err", err)
		fail("rebuild failed: " + err.Error())
		return
	}

	if err := p.store.SetStatus(jobID, store.StatusCompleted, ""); err != nil {
		logger.Error("complete job", "err", err)
		return
	}
	logger.Info("job completed", "chapters", len(result.Chapters))
}

func (p *Pipeline) previewRequested(jobID string) bool {
	job, err := p.store.Get(jobID)
	return err == nil && job.PreviewRequested
}

// publishPreview rebuilds a PDF containing only the first chapter and flips the
// job to preview_ready.
func (p *Pipeline) publishPreview(jobID, bookTitle, chapterTitle string, blocks []model.Block) {
	input := model.RebuildInput{
		Title: bookTitle,
		Chapters: []model.Chapter{
			{Title: chapterTitle, Blocks: blocks},
		},
	}
	if err := p.pdf.Rebuild(context.Background(), input, p.store.PreviewPath(jobID)); err != nil {
		p.logger.Error("preview rebuild failed", "job", jobID, "err", err)
		return
	}
	if err := p.store.SetStatus(jobID, store.StatusPreviewReady, ""); err != nil {
		p.logger.Error("preview status", "job", jobID, "err", err)
	}
}

// processChapters translates a batch of chapters using a worker pool. A failing
// chapter is recorded and the others keep processing; the caller decides the
// final status.
func (p *Pipeline) processChapters(ctx context.Context, jobID string, tasks []chapterTask) bool {
	work := make(chan chapterTask)
	var wg sync.WaitGroup

	for i := 0; i < p.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range work {
				if ctx.Err() != nil {
					return
				}
				blocks, err := p.translator.TranslateChapter(ctx, t.chapter)
				if err != nil {
					p.logger.Error("chapter translation failed", "job", jobID, "chapter", t.index, "err", err)
					_ = p.store.RecordChapterError(jobID, t.index)
					continue
				}
				if err := p.store.RecordChapter(jobID, t.index, blocks); err != nil {
					p.logger.Error("record chapter", "job", jobID, "chapter", t.index, "err", err)
				}
			}
		}()
	}

	go func() {
		defer close(work)
		for _, t := range tasks {
			if ctx.Err() != nil {
				return
			}
			work <- t
		}
	}()

	wg.Wait()

	job, err := p.store.Get(jobID)
	if err != nil {
		return false
	}
	return len(job.FailedChapters) == 0
}

// buildResult assembles the full translated book in original order and rebuilds
// the PDF. Only called after all chapters translated successfully.
func (p *Pipeline) buildResult(jobID string, original []model.Chapter) error {
	job, err := p.store.Get(jobID)
	if err != nil {
		return err
	}
	chapters := make([]model.Chapter, len(original))
	for i, ch := range original {
		if blocks, ok := job.ChapterResults[i]; ok {
			chapters[i] = model.Chapter{Title: ch.Title, Blocks: blocks}
		}
	}
	return p.pdf.Rebuild(context.Background(), model.RebuildInput{
		Title:    job.Title,
		Chapters: chapters,
	}, p.store.ResultPath(jobID))
}
