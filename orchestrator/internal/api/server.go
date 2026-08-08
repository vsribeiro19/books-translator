package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/vribeiro19/books-translator/api/orchestrator/internal/config"
	"github.com/vribeiro19/books-translator/api/orchestrator/internal/pipeline"
	"github.com/vribeiro19/books-translator/api/orchestrator/internal/store"
)

type Server struct {
	cfg      config.Config
	st       *store.Store
	pipeline *pipeline.Pipeline
	mux      *http.ServeMux
	logger   *slog.Logger
}

func NewServer(cfg config.Config, st *store.Store, p *pipeline.Pipeline, logger *slog.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		st:       st,
		pipeline: p,
		mux:      http.NewServeMux(),
		logger:   logger,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /jobs", s.handleCreateJob)
	s.mux.HandleFunc("GET /jobs/{id}", s.handleGetJob)
	s.mux.HandleFunc("GET /jobs/{id}/preview", s.handleGetPreview)
	s.mux.HandleFunc("GET /jobs/{id}/result", s.handleGetResult)
}

func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.logMiddleware(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}
