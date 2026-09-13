package jobs

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/fuel"
)

// StartFuelSurchargeRecalcJob recomputes every active company's fuel
// surcharge rules against the latest fuel_indexes readings. Mirrors
// StartTreasuryForecastJob: runs once immediately, then on the given
// interval, so recommendations aren't stale while waiting for the first
// tick.
func StartFuelSurchargeRecalcJob(ctx context.Context, companies *company.Service, fuelSvc *fuel.Service, interval time.Duration) {
	runFuelSurchargeRecalc(ctx, companies, fuelSvc)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runFuelSurchargeRecalc(ctx, companies, fuelSvc)
			}
		}
	}()
}

func runFuelSurchargeRecalc(ctx context.Context, companies *company.Service, fuelSvc *fuel.Service) {
	list, err := companies.ListCompanies(ctx)
	if err != nil {
		log.Printf("jobs: fuel surcharge recalc could not list companies: %v", err)
		return
	}
	for _, c := range list {
		if !c.IsActive {
			continue
		}
		if err := fuelSvc.RecalculateSurchargeRules(ctx, c.ID); err != nil {
			log.Printf("jobs: fuel surcharge recalc failed for company %s: %v", c.ID, err)
		}
	}
}
