package pdfclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vsribeiro19/books-translator/api/orchestrator/internal/model"
)

// Client talks to the pdf-service (/extract and /rebuild).
type Client struct {
	baseURL string
	timeout time.Duration
	http    *http.Client
}

// New creates a Client for the given base URL and request timeout.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: stripSlash(baseURL),
		timeout: timeout,
		http:    &http.Client{},
	}
}

func stripSlash(u string) string {
	for len(u) > 1 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	return u
}

// cancelBody cancels the request context only when the response body is
// closed. Canceling earlier (right after headers arrive) would abort body
// reads with "context canceled" before callers consume the payload.
type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	var cancel context.CancelFunc
	if c.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if cancel != nil {
		resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	}
	return resp, nil
}

// ExtractError carries the pdf-service error payload for a failed request.
type ExtractError struct {
	Code    string
	Message string
}

func (e *ExtractError) Error() string {
	return e.Message
}

// Extract uploads a PDF file and returns the structured text.
func (c *Client) Extract(ctx context.Context, pdfPath string) (*model.ExtractResult, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("pdf", filepath.Base(pdfPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, http.MethodPost, "/extract", &buf, mw.FormDataContentType())
	if err != nil {
		return nil, fmt.Errorf("extract request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseServiceError(resp)
	}

	var result model.ExtractResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode extract response: %w", err)
	}
	return &result, nil
}

// Rebuild asks the pdf-service to render a translated PDF and writes it to outPath.
func (c *Client) Rebuild(ctx context.Context, input model.RebuildInput, outPath string) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	resp, err := c.do(ctx, http.MethodPost, "/rebuild", bytes.NewReader(body), "application/json")
	if err != nil {
		return fmt.Errorf("rebuild request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return parseServiceError(resp)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

func parseServiceError(resp *http.Response) error {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if payload.Message == "" {
		payload.Message = fmt.Sprintf("pdf-service returned %d", resp.StatusCode)
	}
	return &ExtractError{Code: payload.Error, Message: payload.Message}
}

// IsRejected reports whether the error means the PDF itself is unusable
// (invalid, encrypted, or without extractable text).
func IsRejected(err error) bool {
	_, ok := err.(*ExtractError)
	return ok
}
