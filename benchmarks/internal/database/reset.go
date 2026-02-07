package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func ResetAll(ctx context.Context, serverURL string, databases []string) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	defer httpClient.CloseIdleConnections()

	var errs []error
	for _, db := range databases {
		resetURL := fmt.Sprintf("%s/db/%s/reset", serverURL, db)
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, resetURL, http.NoBody)
		if err != nil {
			errs = append(errs, fmt.Errorf("reset %s: failed to create request: %w", db, err))
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			errs = append(errs, fmt.Errorf("reset %s: %w", db, err))
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("reset %s: unexpected status %d", db, resp.StatusCode))
		}
	}
	return errors.Join(errs...)
}
