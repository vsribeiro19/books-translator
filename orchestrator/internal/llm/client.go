package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// chatMessage is an OpenAI-compatible chat message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Client is an OpenAI-compatible chat-completions client with retry/backoff.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
	http       *http.Client
}

// New creates a chat client for the given provider settings.
func New(baseURL, apiKey, model string, timeout time.Duration, maxRetries int, backoff time.Duration) *Client {
	for len(baseURL) > 1 && baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		timeout:    timeout,
		maxRetries: maxRetries,
		backoff:    backoff,
		http:       &http.Client{},
	}
}

type retryableError struct {
	Status int
	body   []byte
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("llm request failed with status %d: %s", e.Status, trim(e.body))
}

// complete runs a single chat request with retries on transient failures.
func (c *Client) complete(ctx context.Context, messages []chatMessage) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoff * time.Duration(1<<(attempt-1))
			if jitter := time.Duration(rand.Int63n(int64(delay/2) + 1)); jitter > 0 {
				delay += jitter
			}
			if !sleepCtx(ctx, delay) {
				return "", ctx.Err()
			}
		}

		content, err := c.completeOnce(ctx, messages)
		if err == nil {
			return content, nil
		}
		lastErr = err

		var rerr *retryableError
		if errors.As(err, &rerr) {
			if rerr.Status != http.StatusTooManyRequests && (rerr.Status < 500 || rerr.Status > 599) {
				return "", err
			}
			continue
		}
		if err == context.DeadlineExceeded || err == context.Canceled {
			return "", err
		}
		// Transport/timeout errors are retryable.
	}
	return "", fmt.Errorf("chat request failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) completeOnce(ctx context.Context, messages []chatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := func() (context.Context, context.CancelFunc) {
		if c.timeout > 0 {
			return context.WithTimeout(ctx, c.timeout)
		}
		return context.WithCancel(ctx)
	}()
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", &retryableError{Status: resp.StatusCode, body: raw}
	}

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("llm returned empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func trim(b []byte) string {
	s := string(b)
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
