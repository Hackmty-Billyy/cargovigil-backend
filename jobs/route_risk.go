package jobs

import (
	"context"
	"log"
	"time"

	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/routecost"
)

// StartRouteRiskJob re-scores every active route of every active company from
// the trips and frictions of the last routecost.RiskWindowDays days. Closing a
// trip already re-scores its own route on the spot; this job is what keeps the
// scores fresh for routes whose trips are still running (a trip that has been
// sitting "delayed" for three days is risk that nobody explicitly reported).
//
// Same shape as the treasury forecast job: one run at startup so the profiles
// are never stale after a deploy, then on the interval.
func StartRouteRiskJob(ctx context.Context, companies *company.Service, routeCost *routecost.Service, interval time.Duration) {
	runRouteRisk(ctx, companies, routeCost)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRouteRisk(ctx, companies, routeCost)
			}
		}
	}()
}

func runRouteRisk(ctx context.Context, companies *company.Service, routeCost *routecost.Service) {
	list, err := companies.ListCompanies(ctx)
	if err != nil {
		log.Printf("jobs: route risk could not list companies: %v", err)
		return
	}
	for _, c := range list {
		if !c.IsActive {
			continue
		}
		if _, err := routeCost.RecalculateAllRouteRisks(ctx, c.ID); err != nil {
			log.Printf("jobs: route risk recalculation failed for company %s: %v", c.ID, err)
		}
	}
}
