package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryMessageStopsAfterConfiguredAttempts(t *testing.T) {
	attempts := 0
	gotAttempts, err := retryMessage(context.Background(), 5, 0, func() error {
		attempts++
		return errors.New("temporary failure")
	})

	if gotAttempts != 5 || attempts != 5 || err == nil || err.Error() != "temporary failure" {
		t.Fatalf("retryMessage() = (%d, %d, %v), want (5, 5, temporary failure)", gotAttempts, attempts, err)
	}
}

func TestRetryMessageReturnsOnRecovery(t *testing.T) {
	attempts := 0
	gotAttempts, err := retryMessage(context.Background(), 5, 0, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil || gotAttempts != 3 || attempts != 3 {
		t.Fatalf("retryMessage() = (%d, %d, %v), want (3, 3, nil)", gotAttempts, attempts, err)
	}
}

func TestRetryMessageStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	gotAttempts, err := retryMessage(ctx, 5, time.Hour, func() error {
		attempts++
		cancel()
		return errors.New("temporary failure")
	})

	if !errors.Is(err, context.Canceled) || gotAttempts != 1 || attempts != 1 {
		t.Fatalf("retryMessage() = (%d, %d, %v), want (1, 1, context.Canceled)", gotAttempts, attempts, err)
	}
}

func TestRetryDelayCapsWithoutDurationOverflow(t *testing.T) {
	if delay := retryDelay(time.Second, int(^uint(0)>>1)); delay != 30*time.Second {
		t.Fatalf("retryDelay() = %s, want 30s", delay)
	}
}
