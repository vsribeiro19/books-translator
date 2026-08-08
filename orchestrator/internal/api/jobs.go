package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/vribeiro19/books-translator/api/orchestrator/internal/store"
)

// createJobResponse is returned by POST /jobs.
type createJobResponse struct {
	JobID  string          `json:"jobId"`
	Status store.JobStatus `json:"status"`
}

// jobResponse is returned by GET /jobs/{id}.
type jobResponse struct {
	JobID    string          `json:"jobId"`
	Status   store.JobStatus `json:"status"`
	Error    *string         `json:"error,omitempty"`
	Preview  *bool           `json:"previewRequested,omitempty"`
	Progress *progress       `json:"progress,omitempty"`
}

type progress struct {
	ChaptersDone  int `json:"chaptersDone"`
	ChaptersTotal int `json:"chaptersTotal"`
}

type errorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func errorBody(code, message string) errorEnvelope {
	return errorEnvelope{Error: code, Message: message}
}

func newJobID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	limit := s.cfg.Server.MaxUploadBytes
	if limit <= 0 {
		limit = 100 << 20
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_pdf", "expected multipart/form-data with field 'pdf'"))
		return
	}
	multipart := r.MultipartForm
	defer func() {
		if multipart != nil {
			_ = multipart.RemoveAll()
		}
	}()

	file, _, err := r.FormFile("pdf")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_pdf", "missing multipart field 'pdf' with the PDF file"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_pdf", "failed reading upload"))
		return
	}
	if int64(len(data)) > limit {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody("file_too_large", "file exceeds the size limit"))
		return
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		writeJSON(w, http.StatusBadRequest, errorBody("invalid_pdf", "uploaded file is not a PDF"))
		return
	}

	preview := r.FormValue("preview_first_chapter") == "true"

	id, err := newJobID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal", "could not generate job id"))
		return
	}

	job, err := s.st.Create(id, preview)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody("internal", "could not create job"))
		return
	}
	if err := os.WriteFile(s.st.OriginalPath(id), data, 0o644); err != nil {
		_ = s.st.SetStatus(id, store.StatusFailed, "could not store upload")
		writeJSON(w, http.StatusInternalServerError, errorBody("internal", "could not store upload"))
		return
	}

	s.pipeline.Start(id)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(createJobResponse{JobID: job.ID, Status: job.Status})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.st.Get(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("job_not_found", "no such job"))
		return
	}
	resp := jobResponse{
		JobID:   job.ID,
		Status:  job.Status,
		Preview: &job.PreviewRequested,
	}
	if job.Error != "" {
		resp.Error = &job.Error
	}
	resp.Progress = &progress{ChaptersDone: job.ChaptersDone, ChaptersTotal: job.ChaptersTotal}
	writeJSON(w, http.StatusOK, resp)
}

// serveJobFile serves a produced PDF once its job reaches the required status.
func (s *Server) serveJobFile(w http.ResponseWriter, r *http.Request, id, path string, requireStatus store.JobStatus) {
	job, err := s.st.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("job_not_found", "no such job"))
		return
	}

	ready := false
	switch requireStatus {
	case store.StatusPreviewReady:
		ready = job.Status == store.StatusPreviewReady || job.Status == store.StatusCompleted
	case store.StatusCompleted:
		ready = job.Status == store.StatusCompleted
	}
	if !ready {
		writeJSON(w, http.StatusConflict, errorBody("not_ready", "PDF not available yet"))
		return
	}

	if _, err := os.Stat(path); err != nil {
		writeJSON(w, http.StatusConflict, errorBody("not_ready", "PDF not available yet"))
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="translated.pdf"`)
	http.ServeFile(w, r, path)
}

func (s *Server) handleGetPreview(w http.ResponseWriter, r *http.Request) {
	s.serveJobFile(w, r, r.PathValue("id"), s.st.PreviewPath(r.PathValue("id")), store.StatusPreviewReady)
}

func (s *Server) handleGetResult(w http.ResponseWriter, r *http.Request) {
	s.serveJobFile(w, r, r.PathValue("id"), s.st.ResultPath(r.PathValue("id")), store.StatusCompleted)
}
