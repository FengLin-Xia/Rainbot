package llm

import (
	"context"
	"errors"
	"net"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	retryMaxAttempts = 3
	retryBaseDelay   = 500 * time.Millisecond
)

// isRetryable returns true for transient errors worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatusCode == 429 || apiErr.HTTPStatusCode >= 500
	}
	return false
}

// withRetry calls fn up to retryMaxAttempts times with exponential backoff.
func withRetry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	var err error
	for attempt := 0; attempt < retryMaxAttempts; attempt++ {
		var result T
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if !isRetryable(err) || attempt == retryMaxAttempts-1 {
			return zero, err
		}
		delay := retryBaseDelay << uint(attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	return zero, err
}
