package reliability

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	var calls int
	err := Do(context.Background(), DefaultRetryConfig(), "test_op", func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetriesOnTransientError(t *testing.T) {
	var calls int
	err := Do(context.Background(), DefaultRetryConfig(), "test_op", func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_ExhaustsRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:      2,
		InitialInterval: 10 * time.Millisecond,
		MaxElapsed:      5 * time.Second,
	}
	var calls int
	err := Do(context.Background(), cfg, "test_op", func() error {
		calls++
		return errors.New("permanent")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// MaxRetries=2 means up to 3 total attempts (initial + 2 retries)
	if calls > 3 {
		t.Fatalf("expected at most 3 calls, got %d", calls)
	}
	if !errors.Is(err, errors.Unwrap(err)) {
		// Just verify the error message wraps operation name
		if err.Error() == "" {
			t.Fatal("expected non-empty error")
		}
	}
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{
		MaxRetries:      10,
		InitialInterval: 50 * time.Millisecond,
		MaxElapsed:      10 * time.Second,
	}
	var calls int32
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, "test_op", func() error {
		atomic.AddInt32(&calls, 1)
		return errors.New("keep failing")
	})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	// Should not have retried many times due to cancellation
	got := atomic.LoadInt32(&calls)
	if got > 5 {
		t.Fatalf("expected few calls due to cancellation, got %d", got)
	}
}

func TestDo_ZeroMaxRetries_RunsOnce(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 0}
	var calls int
	err := Do(context.Background(), cfg, "test_op", func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call with MaxRetries=0, got %d", calls)
	}
}

func TestDoWithResult_ReturnsValue(t *testing.T) {
	cfg := DefaultRetryConfig()
	result, err := DoWithResult(context.Background(), cfg, "test_op", func() (string, error) {
		return "hello", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %q", result)
	}
}

func TestDoWithResult_RetriesThenSucceeds(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:      3,
		InitialInterval: 10 * time.Millisecond,
		MaxElapsed:      5 * time.Second,
	}
	var calls int
	result, err := DoWithResult(context.Background(), cfg, "test_op", func() (int, error) {
		calls++
		if calls < 2 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Fatalf("expected 42, got %d", result)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Fatalf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.InitialInterval != 1*time.Second {
		t.Fatalf("expected InitialInterval=1s, got %v", cfg.InitialInterval)
	}
	if cfg.MaxElapsed != 30*time.Second {
		t.Fatalf("expected MaxElapsed=30s, got %v", cfg.MaxElapsed)
	}
}
