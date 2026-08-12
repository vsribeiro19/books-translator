package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/model"
)

// JobStatus is the lifecycle state of a translation job.
type JobStatus string

const (
	StatusPending      JobStatus = "pending"
	StatusProcessing   JobStatus = "processing"
	StatusPreviewReady JobStatus = "preview_ready"
	StatusCompleted    JobStatus = "completed"
	StatusFailed       JobStatus = "failed"
)

var (
	// ErrNotFound is returned when a job id does not exist.
	ErrNotFound = errors.New("job not found")
	// ErrDone is returned when trying to transition a terminal job.
	ErrDone = errors.New("job already in a terminal state")
)

// Job is the persisted record of a translation job.
type Job struct {
	ID               string                `json:"id"`
	Status           JobStatus             `json:"status"`
	Error            string                `json:"error,omitempty"`
	PreviewRequested bool                  `json:"previewRequested"`
	Title            string                `json:"title,omitempty"`
	PageCount        int                   `json:"pageCount,omitempty"`
	ChaptersDone     int                   `json:"chaptersDone"`
	ChaptersTotal    int                   `json:"chaptersTotal"`
	ChapterResults   map[int][]model.Block `json:"chapterResults,omitempty"`
	FailedChapters   []int                 `json:"failedChapters,omitempty"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

// Store keeps jobs in memory and persists each job as JSON in dataDir.
type Store struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	dataDir string
}

// New creates a Store backed by dataDir (created if missing).
func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{
		jobs:    make(map[string]*Job),
		dataDir: dataDir,
	}, nil
}

// jobDir returns the directory holding all files for a job.
func (s *Store) jobDir(id string) string {
	return filepath.Join(s.dataDir, id)
}

// StatePath returns the path to the job's JSON state file.
func (s *Store) StatePath(id string) string {
	return filepath.Join(s.jobDir(id), "state.json")
}

// OriginalPath returns the path to the uploaded PDF.
func (s *Store) OriginalPath(id string) string {
	return filepath.Join(s.jobDir(id), "original.pdf")
}

// PreviewPath returns the path of the partial (first chapter) PDF, when enabled.
func (s *Store) PreviewPath(id string) string {
	return filepath.Join(s.jobDir(id), "preview.pdf")
}

// ResultPath returns the path of the final translated PDF.
func (s *Store) ResultPath(id string) string {
	return filepath.Join(s.jobDir(id), "result.pdf")
}

// Create registers a new job and persists it.
func (s *Store) Create(id string, preview bool) (*Job, error) {
	now := time.Now().UTC()
	job := &Job{
		ID:               id,
		Status:           StatusPending,
		PreviewRequested: preview,
		ChapterResults:   make(map[int][]model.Block),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := os.MkdirAll(s.jobDir(id), 0o755); err != nil {
		return nil, fmt.Errorf("create job dir: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id] = job
	return job, s.persistLocked(job)
}

// Get returns a copy of the job by id.
func (s *Store) Get(id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *job
	return &cp, nil
}

// SetStatus transitions the job and persists it. Terminal states reject
// further transitions. Returns ErrDone if the transition is illegal.
func (s *Store) SetStatus(id string, status JobStatus, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if job.Status == StatusCompleted || job.Status == StatusFailed {
		return ErrDone
	}
	job.Status = status
	if errMsg != "" {
		job.Error = errMsg
	}
	job.UpdatedAt = time.Now().UTC()
	return s.persistLocked(job)
}

// SetInfo records extraction metadata (title, page count, chapters).
func (s *Store) SetInfo(id, title string, pageCount, chaptersTotal int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	job.Title = title
	job.PageCount = pageCount
	job.ChaptersTotal = chaptersTotal
	job.UpdatedAt = time.Now().UTC()
	return s.persistLocked(job)
}

// RecordChapter stores the translated blocks for a chapter and bumps progress.
func (s *Store) RecordChapter(id string, index int, blocks []model.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	job.ChapterResults[index] = blocks
	job.ChaptersDone++
	job.UpdatedAt = time.Now().UTC()
	return s.persistLocked(job)
}

// RecordChapterError marks a chapter as failed.
func (s *Store) RecordChapterError(id string, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ErrNotFound
	}
	job.FailedChapters = append(job.FailedChapters, index)
	job.UpdatedAt = time.Now().UTC()
	return s.persistLocked(job)
}

// Recover reloads persisted jobs from disk and marks interrupted ones as failed.
// Pending/processing jobs (a server restart happened mid-run) are failed with
// a descriptive message and are NOT resumed (MVP).
func (s *Store) Recover() error {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := s.StatePath(e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		if job.Status == StatusPending || job.Status == StatusProcessing {
			job.Status = StatusFailed
			job.Error = "server restarted before the job finished; no resume in MVP"
			job.UpdatedAt = time.Now().UTC()
		}
		s.jobs[e.Name()] = &job
		_ = s.persistLocked(&job)
	}
	return nil
}

func (s *Store) persistLocked(job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	path := s.StatePath(job.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
