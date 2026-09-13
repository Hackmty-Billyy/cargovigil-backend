package jobs

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/treasury"
)

// StartTreasuryForecastJob recalculates the 60-day cash flow projection for
// every active company. It runs once immediately (so the forecast isn't
// empty while waiting for the first tick) and then on the given interval.
func StartTreasuryForecastJob(ctx context.Context, companies *company.Service, treasurySvc *treasury.Service, interval time.Duration) {
	runTreasuryForecast(ctx, companies, treasurySvc)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runTreasuryForecast(ctx, companies, treasurySvc)
			}
		}
	}()
}

func runTreasuryForecast(ctx context.Context, companies *company.Service, treasurySvc *treasury.Service) {
	list, err := companies.ListCompanies(ctx)
	if err != nil {
		log.Printf("jobs: treasury forecast could not list companies: %v", err)
		return
	}
	for _, c := range list {
		if !c.IsActive {
			continue
		}
		if err := treasurySvc.RecalculateForecast(ctx, c.ID); err != nil {
			log.Printf("jobs: treasury forecast failed for company %s: %v", c.ID, err)
		}
	}
}
