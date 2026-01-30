package database

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ResetAll resets all specified databases via HTTP DELETE requests.
func ResetAll(ctx context.Context, serverURL string, databases []string) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	for _, db := range databases {
		resetURL := fmt.Sprintf("%s/db/%s/reset", serverURL, db)
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, resetURL, http.NoBody)
		if err != nil {
			return fmt.Errorf("reset %s: failed to create request: %w", db, err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("reset %s: %w", db, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("reset %s: unexpected status %d", db, resp.StatusCode)
		}
	}
	return nil
}
