// SPDX-License-Identifier: Apache-2.0
// Copyright Daniel Neumann

package controller

import (
	"context"
	"fmt"
	"time"
)

func withRetry(ctx context.Context, attempts int, backoff time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 1; i <= attempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if i == attempts {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", attempts, lastErr)
}
