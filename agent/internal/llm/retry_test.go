package llm

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

func init() {
	retryBaseDelay = time.Millisecond // speed up retry tests
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context_canceled", context.Canceled, false},
		{"context_deadline", context.DeadlineExceeded, false},
		{"net_error", &net.OpError{Op: "dial"}, true},
		{"http_429", &openai.APIError{HTTPStatusCode: 429}, true},
		{"http_500", &openai.APIError{HTTPStatusCode: 500}, true},
		{"http_503", &openai.APIError{HTTPStatusCode: 503}, true},
		{"http_400", &openai.APIError{HTTPStatusCode: 400}, false},
		{"http_401", &openai.APIError{HTTPStatusCode: 401}, false},
		{"generic", errors.New("something"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRetryable(c.err); got != c.want {
				t.Errorf("isRetryable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, err := withRetry(context.Background(), func() (int, error) {
		calls++
		return 42, nil
	})
	if err != nil || result != 42 || calls != 1 {
		t.Errorf("got (%d, %v) in %d calls; want (42, nil, 1)", result, err, calls)
	}
}

func TestWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	calls := 0
	result, err := withRetry(context.Background(), func() (int, error) {
		calls++
		if calls < 2 {
			return 0, &openai.APIError{HTTPStatusCode: 500}
		}
		return 99, nil
	})
	if err != nil || result != 99 || calls != 2 {
		t.Errorf("got (%d, %v) in %d calls; want (99, nil, 2)", result, err, calls)
	}
}

func TestWithRetry_ExhaustsMaxAttempts(t *testing.T) {
	calls := 0
	_, err := withRetry(context.Background(), func() (int, error) {
		calls++
		return 0, &openai.APIError{HTTPStatusCode: 500}
	})
	if err == nil {
		t.Error("expected error after exhausting retries, got nil")
	}
	if calls != retryMaxAttempts {
		t.Errorf("calls = %d, want %d (retryMaxAttempts)", calls, retryMaxAttempts)
	}
}

func TestWithRetry_NonRetryableStopsImmediately(t *testing.T) {
	calls := 0
	_, err := withRetry(context.Background(), func() (int, error) {
		calls++
		return 0, &openai.APIError{HTTPStatusCode: 400}
	})
	if err == nil {
		t.Error("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (non-retryable must not retry)", calls)
	}
}

func TestWithRetry_ContextCancelStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := withRetry(ctx, func() (int, error) {
		calls++
		cancel()
		return 0, &openai.APIError{HTTPStatusCode: 500}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
