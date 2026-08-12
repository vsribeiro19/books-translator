package main

import (
	"log/slog"
	"os"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/api"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/config"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/llm"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/pdfclient"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/pipeline"
	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	st, err := store.New(cfg.Storage.DataDir)
	if err != nil {
		slog.Error("init store", "err", err)
		os.Exit(1)
	}
	if err := st.Recover(); err != nil {
		slog.Error("recover store", "err", err)
		os.Exit(1)
	}

	name, provider, ok := cfg.DefaultProvider()
	var client *llm.Client
	if ok {
		if provider.APIKey() == "" {
			logger.Warn("no api key set for default provider", "provider", name)
		}
		client = llm.New(
			provider.BaseURL,
			provider.APIKey(),
			provider.Model,
			cfg.Translation.RequestTimeout.Duration,
			cfg.Translation.MaxRetries,
			cfg.Translation.RetryBackoff.Duration,
		)
		logger.Info("llm provider configured",
			"provider", name,
			"model", provider.Model,
			"base_url", provider.BaseURL,
			"has_api_key", provider.APIKey() != "",
		)
	} else {
		logger.Error("no llm provider configured; translation will fail")
		client = llm.New("", "", "", 0, 0, 0)
	}

	translator := llm.NewTranslator(client, cfg.Translation.ChunkTokens, cfg.Translation.ContextChunks)
	pc := pdfclient.New(cfg.PDFService.URL, cfg.PDFService.RequestTimeout.Duration)
	p := pipeline.New(st, pc, translator, cfg.Translation.Concurrency, logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	srv := api.NewServer(cfg, st, p, logger)
	logger.Info("orchestrator listening", "addr", addr)

	if err := srv.Run(addr); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
