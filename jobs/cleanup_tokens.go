package jobs

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/auth"
)

// StartRefreshTokenCleanup periodically purges expired/revoked refresh
// tokens so the table doesn't grow unbounded. Runs until ctx is cancelled.
func StartRefreshTokenCleanup(ctx context.Context, repo auth.RefreshTokenRepository, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := repo.DeleteExpiredBefore(ctx, time.Now())
				if err != nil {
					log.Printf("jobs: refresh token cleanup failed: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("jobs: purged %d expired/revoked refresh tokens", n)
				}
			}
		}
	}()
}
