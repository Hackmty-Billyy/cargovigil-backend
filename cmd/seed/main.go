// Command seed populates every table with realistic bulk demo data for each
// active client company, so the whole system can be tested end-to-end with
// real volume instead of the handful of fixture rows from the dev
// migrations. It is a manual tool (`go run ./cmd/seed`), not part of the
// server's automatic migration chain — safe to re-run, each run just adds
// another batch of trips and their related records.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/uptrace/bun"

	"github.com/Hackmty-Billyy/cargovigil-backend/config"
	"github.com/Hackmty-Billyy/cargovigil-backend/database"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/company"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/fuel"
	"github.com/Hackmty-Billyy/cargovigil-backend/domain/treasury"
)

const tripsPerCompany = 25

var vehicleTypes = []string{"truck", "ship", "plane"}
var fuelTypes = map[string]string{"truck": "diesel", "ship": "bunker_c", "plane": "jet_a1"}
var cargoTypes = []string{"Bobinas de Acero", "Contenedores Refrigerados", "Autopartes", "Bebidas Embotelladas", "Electrónicos", "Carga Farmacéutica", "Maquinaria Pesada", "Textiles"}
var frictionEvents = []string{"port_demurrage", "customs_delay", "traffic_congestion", "mechanical_failure", "route_deviation", "weather_hazard", "warehouse_detention"}
var expenseCategories = []string{"fuel", "toll", "maintenance", "driver_payroll", "port_fees", "insurance"}

type catalog struct {
	vehicleIDs  []string
	routeIDs    []string
	clientIDs   []string
	contractIDs map[string]string // clientID -> contractID (best-effort, may be empty)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	db := database.NewBunDB(pool)

	companyRepo := company.NewBunCompanyRepository(db)
	vehicleRepo := company.NewBunVehicleRepository(db)
	routeRepo := company.NewBunRouteRepository(db)
	clientRepo := company.NewBunClientRepository(db)
	contractRepo := company.NewBunContractRepository(db)
	treasuryRepo := treasury.NewBunRepository(db)
	treasuryService := treasury.NewService(treasuryRepo, treasuryRepo, treasuryRepo, treasuryRepo, treasuryRepo)

	companies, err := companyRepo.ListAll(ctx)
	if err != nil {
		log.Fatalf("list companies: %v", err)
	}

	for _, comp := range companies {
		if !comp.IsActive || comp.Name == "CargoVigil Platform" {
			continue
		}
		fmt.Printf("== Seeding %s (%s) ==\n", comp.Name, comp.ID)

		cat := ensureBaselineCatalog(ctx, comp.ID, vehicleRepo, routeRepo, clientRepo, contractRepo)
		ensureBankAccounts(ctx, db, comp.ID)
		ensureRouteRiskProfiles(ctx, db, comp.ID, cat.routeIDs)
		ensureClientPaymentBehavior(ctx, db, comp.ID, cat.clientIDs)

		bankAccountIDs := bankAccountIDsFor(ctx, db, comp.ID)

		seedTrips(ctx, db, treasuryRepo, comp.ID, cat, bankAccountIDs)
		seedLiquidityStress(ctx, db, treasuryRepo, comp.ID)

		if err := treasuryService.RecalculateForecast(ctx, comp.ID); err != nil {
			log.Printf("  forecast recalculation failed: %v", err)
		} else {
			fmt.Println("  forecast recalculated")
		}
	}

	fmt.Println("Done.")
}

// ---- Catalog baseline ----

func ensureBaselineCatalog(ctx context.Context, companyID string, vehicleRepo *company.BunVehicleRepository,
	routeRepo *company.BunRouteRepository, clientRepo *company.BunClientRepository,
	contractRepo *company.BunContractRepository) catalog {

	suffix := time.Now().UnixNano() % 100000

	for i := 0; i < 3; i++ {
		v := &company.Vehicle{
			CompanyID:  companyID,
			Type:       vehicleTypes[i%len(vehicleTypes)],
			Identifier: fmt.Sprintf("SEED-VEH-%d-%d", suffix, i),
		}
		if err := vehicleRepo.Create(ctx, v); err != nil {
			log.Printf("  vehicle create failed: %v", err)
		}
	}

	routePairs := [][2]string{
		{"Monterrey, NL", "Nuevo Laredo, TAMPS"},
		{"Puerto de Veracruz, VER", "Port of Houston, TX"},
		{"CDMX", "Guadalajara, JAL"},
	}
	for i, pair := range routePairs {
		dist := 200.0 + float64(i)*350.0
		r := &company.Route{CompanyID: companyID, Origin: pair[0], Destination: pair[1], DistanceKM: &dist}
		if err := routeRepo.Create(ctx, r); err != nil {
			log.Printf("  route create failed: %v", err)
		}
	}

	clientNames := []string{
		fmt.Sprintf("Cliente Industrial %d S.A. de C.V.", suffix),
		fmt.Sprintf("Grupo Logístico %d", suffix),
		fmt.Sprintf("Comercializadora %d", suffix),
	}
	for i, name := range clientNames {
		taxID := fmt.Sprintf("SED%06d%03d", suffix, i)
		cl := &company.Client{CompanyID: companyID, Name: name, TaxID: &taxID}
		if err := clientRepo.Create(ctx, cl); err != nil {
			log.Printf("  client create failed: %v", err)
			continue
		}
		starts := time.Now().AddDate(0, -6, 0)
		ends := time.Now().AddDate(1, 0, 0)
		ct := &company.Contract{
			CompanyID: companyID, ClientID: cl.ID,
			Reference: fmt.Sprintf("CTR-SEED-%d-%d", suffix, i),
			StartsOn:  &starts, EndsOn: &ends,
		}
		if err := contractRepo.Create(ctx, ct); err != nil {
			log.Printf("  contract create failed: %v", err)
		}
	}

	vehicles, _ := vehicleRepo.ListByCompany(ctx, companyID)
	routes, _ := routeRepo.ListByCompany(ctx, companyID)
	clients, _ := clientRepo.ListByCompany(ctx, companyID)
	contracts, _ := contractRepo.ListByCompany(ctx, companyID)

	cat := catalog{contractIDs: map[string]string{}}
	for _, v := range vehicles {
		cat.vehicleIDs = append(cat.vehicleIDs, v.ID)
	}
	for _, r := range routes {
		cat.routeIDs = append(cat.routeIDs, r.ID)
	}
	for _, cl := range clients {
		cat.clientIDs = append(cat.clientIDs, cl.ID)
	}
	for _, ct := range contracts {
		cat.contractIDs[ct.ClientID] = ct.ID
	}
	return cat
}

func ensureBankAccounts(ctx context.Context, db *bun.DB, companyID string) {
	count, err := db.NewSelect().Model((*treasury.BankAccount)(nil)).Where("company_id = ?", companyID).Count(ctx)
	if err != nil {
		log.Printf("  count bank_accounts failed: %v", err)
		return
	}
	if count > 0 {
		return
	}
	accounts := []treasury.BankAccount{
		{CompanyID: companyID, BankName: "BBVA Bancomer Operaciones", AccountNumberMask: "**** 7731", Currency: "MXN", CurrentBalance: 900000.00, MinimumRequiredBalance: 250000.00, IsActive: true},
		{CompanyID: companyID, BankName: "Santander Tesorería USD", AccountNumberMask: "**** 5510", Currency: "USD", CurrentBalance: 40000.00, MinimumRequiredBalance: 10000.00, IsActive: true},
	}
	if _, err := db.NewInsert().Model(&accounts).Exec(ctx); err != nil {
		log.Printf("  bank_accounts seed failed: %v", err)
	}
}

func bankAccountIDsFor(ctx context.Context, db *bun.DB, companyID string) []string {
	var accounts []treasury.BankAccount
	err := db.NewSelect().Model(&accounts).Column("id").
		Where("company_id = ?", companyID).
		Where("currency = ?", "MXN").
		Scan(ctx)
	if err != nil {
		return nil
	}
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return ids
}

func ensureRouteRiskProfiles(ctx context.Context, db *bun.DB, companyID string, routeIDs []string) {
	for _, routeID := range routeIDs {
		rrp := &RouteRiskProfile{
			CompanyID: companyID, RouteID: routeID,
			HistoricalRiskScore: randFloat(1.0, 1.6), AvgDelayHours: randFloat(1.0, 10.0),
			SuggestedContingencyPercentage: randFloat(3.0, 10.0), IncidentCount: randInt(0, 15),
		}
		_, err := db.NewInsert().Model(rrp).On("CONFLICT (company_id, route_id) DO NOTHING").Exec(ctx)
		if err != nil {
			log.Printf("  route_risk_profiles seed failed: %v", err)
		}
	}
}

func ensureClientPaymentBehavior(ctx context.Context, db *bun.DB, companyID string, clientIDs []string) {
	for _, clientID := range clientIDs {
		cpb := &ClientPaymentBehavior{
			CompanyID: companyID, ClientID: clientID,
			AveragePODApprovalDays: randFloat(1.0, 8.0), AveragePaymentDelayDays: randFloat(0.0, 6.0),
			DisputeRatePercentage: randFloat(0.0, 8.0),
		}
		_, err := db.NewInsert().Model(cpb).On("CONFLICT (company_id, client_id) DO NOTHING").Exec(ctx)
		if err != nil {
			log.Printf("  client_payment_behavior seed failed: %v", err)
		}
	}
}

// ---- Trips and everything hanging off a trip ----

func seedTrips(ctx context.Context, db *bun.DB, treasuryRepo *treasury.BunRepository,
	companyID string, cat catalog, mxnBankAccountIDs []string) {

	if len(cat.vehicleIDs) == 0 || len(cat.routeIDs) == 0 || len(cat.clientIDs) == 0 {
		log.Printf("  skipping trips: catalog incomplete for company %s", companyID)
		return
	}

	runTag := time.Now().UnixNano() % 1000000
	statuses := weightedStatuses()

	for i := 0; i < tripsPerCompany; i++ {
		vehicleID := randChoice(cat.vehicleIDs)
		routeID := randChoice(cat.routeIDs)
		clientID := randChoice(cat.clientIDs)
		status := statuses[i%len(statuses)]

		vehicleType := "truck"
		veh := new(company.Vehicle)
		if err := db.NewSelect().Model(veh).Column("type").Where("id = ?", vehicleID).Scan(ctx); err == nil {
			vehicleType = veh.Type
		}

		departure, estimatedArrival, actualArrival := datesForStatus(status)
		freight := freightFor(vehicleType)
		fuelSurcharge := freight * randFloat(0.03, 0.09)
		contingency := freight * randFloat(0.02, 0.08)
		weight := randFloat(2, 4500)

		var contractID *string
		if cid, ok := cat.contractIDs[clientID]; ok {
			contractID = &cid
		}

		trip := &Trip{
			CompanyID: companyID, VehicleID: vehicleID, RouteID: routeID, ClientID: clientID, ContractID: contractID,
			TrackingCode: fmt.Sprintf("SEED-%d-%03d", runTag, i), CargoType: randChoice(cargoTypes), CargoWeightTons: weight,
			Status: status, DepartureDate: departure, EstimatedArrivalDate: estimatedArrival, ActualArrivalDate: actualArrival,
			AgreedFreightPrice: freight, Currency: "USD", FuelSurchargeAmount: fuelSurcharge, ContingencyBudget: contingency,
		}
		if _, err := db.NewInsert().Model(trip).Returning("id").Exec(ctx); err != nil {
			log.Printf("  trip insert failed: %v", err)
			continue
		}
		tripID := trip.ID

		switch status {
		case "completed":
			seedFriction(ctx, db, companyID, tripID, departure, 0.35)
			seedFuelLog(ctx, db, companyID, tripID, vehicleType, departure)
			pod := seedPOD(ctx, db, companyID, tripID, clientID, actualArrival, 0.15)
			invoiceID, total := seedInvoice(ctx, treasuryRepo, companyID, tripID, clientID, departure, freight+fuelSurcharge, pod)
			payIfFunds(ctx, treasuryRepo, companyID, invoiceID, total, mxnBankAccountIDs, 0.7)
			totalExpenses := seedExpenses(ctx, treasuryRepo, companyID, tripID, departure, freight, mxnBankAccountIDs, 0.8)
			seedProfitability(ctx, db, companyID, tripID, freight+fuelSurcharge, totalExpenses)

		case "delayed":
			seedFriction(ctx, db, companyID, tripID, departure, 1.0)
			seedFuelLog(ctx, db, companyID, tripID, vehicleType, departure)
			pod := seedPOD(ctx, db, companyID, tripID, clientID, nil, 0.4)
			invoiceID, total := seedInvoice(ctx, treasuryRepo, companyID, tripID, clientID, departure, freight+fuelSurcharge, pod)
			payIfFunds(ctx, treasuryRepo, companyID, invoiceID, total, mxnBankAccountIDs, 0.2)
			seedExpenses(ctx, treasuryRepo, companyID, tripID, departure, freight, mxnBankAccountIDs, 0.3)

		case "in_transit":
			seedFuelLog(ctx, db, companyID, tripID, vehicleType, departure)
			seedInvoice(ctx, treasuryRepo, companyID, tripID, clientID, departure, freight+fuelSurcharge, false)
			seedExpenses(ctx, treasuryRepo, companyID, tripID, departure, freight, mxnBankAccountIDs, 0.1)

		case "scheduled":
			seedInvoice(ctx, treasuryRepo, companyID, tripID, clientID, departure, freight+fuelSurcharge, false)

		case "cancelled":
			// no financial trail — matches a trip that never actually ran.
		}
	}

	fmt.Printf("  %d trips seeded\n", tripsPerCompany)
}

func seedFriction(ctx context.Context, db *bun.DB, companyID, tripID string, departure time.Time, prob float64) {
	if rand.Float64() > prob {
		return
	}
	start := departure.Add(time.Duration(randInt(2, 20)) * time.Hour)
	duration := randFloat(1.0, 8.0)
	end := start.Add(time.Duration(duration * float64(time.Hour)))
	tf := &TripFriction{
		CompanyID: companyID, TripID: tripID, EventType: randChoice(frictionEvents), LocationName: "Punto de control en ruta",
		StartedAt: start, EndedAt: &end, DurationHours: duration, CostImpact: duration * randFloat(20, 60),
		Notes: "Generado por seeder de simulación",
	}
	if _, err := db.NewInsert().Model(tf).Exec(ctx); err != nil {
		log.Printf("  trip_frictions insert failed: %v", err)
	}
}

func seedFuelLog(ctx context.Context, db *bun.DB, companyID, tripID, vehicleType string, departure time.Time) {
	fuelType := fuelTypes[vehicleType]
	if fuelType == "" {
		fuelType = "diesel"
	}
	volume := randFloat(60, 200)
	costPerUnit := randFloat(1.1, 1.4)
	if fuelType == "bunker_c" {
		costPerUnit = randFloat(580, 650)
		volume = randFloat(20, 45)
	}
	odo := randFloat(50, 2000)
	purchasedAt := departure.Add(2 * time.Hour)
	l := &fuel.TripFuelLog{
		CompanyID: companyID, TripID: tripID, FuelType: fuelType, VolumePurchased: volume, CostPerUnit: costPerUnit,
		TotalCost: volume * costPerUnit, OdometerOrHours: &odo, PurchasedAt: purchasedAt,
	}
	if _, err := db.NewInsert().Model(l).Exec(ctx); err != nil {
		log.Printf("  trip_fuel_logs insert failed: %v", err)
	}
}

// seedPOD returns true if the POD ended up under dispute (used to push the
// invoice's adjusted_due_date out, mirroring the real POD -> treasury link).
func seedPOD(ctx context.Context, db *bun.DB, companyID, tripID, clientID string, actualArrival *time.Time, disputeProb float64) bool {
	disputed := rand.Float64() < disputeProb
	status := "approved_by_client"
	var deliveryDate, signatureDate *time.Time
	daysToSign := randInt(1, 3)

	if actualArrival != nil {
		d := *actualArrival
		deliveryDate = &d
		s := d.Add(time.Duration(daysToSign) * 24 * time.Hour)
		signatureDate = &s
	}
	var disputeReason *string
	if disputed {
		status = "under_dispute"
		reason := "Discrepancia en cantidad recibida, en revisión"
		disputeReason = &reason
		daysToSign = randInt(8, 20)
		signatureDate = nil
	}

	pod := &PODDocument{
		CompanyID: companyID, TripID: tripID, ClientID: clientID,
		DocumentURL: fmt.Sprintf("https://storage.cargovigil.test/pods/%s.pdf", tripID),
		Status:      status, DisputeReason: disputeReason, DeliveryDate: deliveryDate, SignatureDate: signatureDate, DaysToSign: daysToSign,
	}
	if _, err := db.NewInsert().Model(pod).Exec(ctx); err != nil {
		log.Printf("  pod_documents insert failed: %v", err)
	}
	return disputed
}

func seedInvoice(ctx context.Context, treasuryRepo *treasury.BunRepository,
	companyID, tripID, clientID string, departure time.Time, amount float64, podDisputed bool) (string, float64) {

	issueDate := departure.AddDate(0, 0, 1)
	dueDate := issueDate.AddDate(0, 0, 30)
	adjustedDueDate := dueDate
	if podDisputed {
		adjustedDueDate = dueDate.AddDate(0, 0, randInt(8, 15))
	}

	invoiceNumber := fmt.Sprintf("SEED-INV-%s", randomHex(8))

	inv := &treasury.Invoice{
		CompanyID: companyID, TripID: &tripID, ClientID: clientID, InvoiceNumber: invoiceNumber,
		IssueDate: issueDate, DueDate: dueDate, AdjustedDueDate: adjustedDueDate, TotalAmount: amount,
	}
	if err := treasuryRepo.CreateInvoice(ctx, inv); err != nil {
		log.Printf("  invoice insert failed: %v", err)
		return "", 0
	}
	return inv.ID, amount
}

func payIfFunds(ctx context.Context, treasuryRepo *treasury.BunRepository, companyID, invoiceID string, amount float64, mxnBankAccountIDs []string, prob float64) {
	if invoiceID == "" || len(mxnBankAccountIDs) == 0 || rand.Float64() > prob {
		return
	}
	bankAccountID := randChoice(mxnBankAccountIDs)
	if _, err := treasuryRepo.RecordPayment(ctx, companyID, invoiceID, amount, bankAccountID); err != nil {
		log.Printf("  invoice payment failed: %v", err)
	}
}

func seedExpenses(ctx context.Context, treasuryRepo *treasury.BunRepository,
	companyID, tripID string, departure time.Time, freight float64, mxnBankAccountIDs []string, payProb float64) float64 {

	count := randInt(1, 3)
	var total float64
	for i := 0; i < count; i++ {
		category := randChoice(expenseCategories)
		amount := freight * randFloat(0.02, 0.12)
		dueDate := departure.AddDate(0, 0, randInt(5, 20))

		e := &treasury.Expense{
			CompanyID: companyID, TripID: &tripID, Category: category,
			Description: fmt.Sprintf("%s asociado al viaje", category), Amount: amount, DueDate: dueDate,
		}
		if err := treasuryRepo.CreateExpense(ctx, e); err != nil {
			log.Printf("  expense insert failed: %v", err)
			continue
		}
		total += amount

		if len(mxnBankAccountIDs) > 0 && rand.Float64() < payProb {
			bankAccountID := randChoice(mxnBankAccountIDs)
			if _, err := treasuryRepo.MarkPaid(ctx, companyID, e.ID, bankAccountID); err != nil {
				log.Printf("  expense payment failed: %v", err)
			}
		}
	}
	return total
}

func seedProfitability(ctx context.Context, db *bun.DB, companyID, tripID string, grossRevenue, totalExpenses float64) {
	fuelCost := grossRevenue * randFloat(0.05, 0.15)
	tollPort := grossRevenue * randFloat(0.01, 0.05)
	driverCost := grossRevenue * randFloat(0.03, 0.08)
	frictionCost := grossRevenue * randFloat(0.0, 0.03)
	maintenance := grossRevenue * randFloat(0.01, 0.04)
	totalCost := fuelCost + tollPort + driverCost + frictionCost + maintenance + totalExpenses*0.1
	netProfit := grossRevenue - totalCost
	margin := 0.0
	if grossRevenue > 0 {
		margin = (netProfit / grossRevenue) * 100
	}
	profitPerKM := netProfit / randFloat(150, 2000)

	tp := &TripProfitability{
		CompanyID: companyID, TripID: tripID, GrossRevenue: grossRevenue, FuelCost: fuelCost, TollAndPortCost: tollPort,
		DriverCost: driverCost, FrictionCost: frictionCost, MaintenanceAllocation: maintenance, TotalCost: totalCost,
		NetProfit: netProfit, ProfitMarginPercentage: margin, ProfitPerKM: profitPerKM,
	}
	_, err := db.NewInsert().Model(tp).On("CONFLICT (trip_id) DO NOTHING").Exec(ctx)
	if err != nil {
		log.Printf("  trip_profitability insert failed: %v", err)
	}
}

// seedLiquidityStress guarantees at least one real projected deficit per
// company, sized off its actual MXN balance/threshold, so the alert
// pipeline (cash_alerts) gets genuinely exercised instead of only ever
// showing the one hand-seeded example alert from migration 000017.
func seedLiquidityStress(ctx context.Context, db *bun.DB, treasuryRepo *treasury.BunRepository, companyID string) {
	var accounts []treasury.BankAccount
	err := db.NewSelect().Model(&accounts).
		Where("company_id = ?", companyID).
		Where("currency = ?", "MXN").
		Where("is_active = true").
		Scan(ctx)
	if err != nil {
		return
	}
	var balance, threshold float64
	for _, a := range accounts {
		balance += a.CurrentBalance
		threshold += a.MinimumRequiredBalance
	}
	if balance <= threshold {
		return
	}

	headroom := balance - threshold
	dueDate1 := time.Now().AddDate(0, 0, randInt(4, 7))
	dueDate2 := time.Now().AddDate(0, 0, randInt(8, 12))

	expenses := []treasury.Expense{
		{CompanyID: companyID, Category: "port_fees", Description: "Liquidación consolidada de puerto (simulación de estrés)", Amount: headroom * 0.7, DueDate: dueDate1},
		{CompanyID: companyID, Category: "maintenance", Description: "Mantenimiento mayor de flota (simulación de estrés)", Amount: headroom * 0.6, DueDate: dueDate2},
	}
	for i := range expenses {
		if err := treasuryRepo.CreateExpense(ctx, &expenses[i]); err != nil {
			log.Printf("  liquidity stress expense failed: %v", err)
		}
	}
}

// ---- date & random helpers ----

func weightedStatuses() []string {
	s := []string{}
	for i := 0; i < 10; i++ {
		switch {
		case i < 4:
			s = append(s, "completed")
		case i < 6:
			s = append(s, "in_transit")
		case i < 8:
			s = append(s, "scheduled")
		case i < 9:
			s = append(s, "delayed")
		default:
			s = append(s, "cancelled")
		}
	}
	return s
}

func datesForStatus(status string) (departure, estimatedArrival time.Time, actualArrival *time.Time) {
	now := time.Now()
	switch status {
	case "completed":
		departure = now.AddDate(0, 0, -randInt(5, 90))
		estimatedArrival = departure.AddDate(0, 0, randInt(1, 5))
		a := estimatedArrival.Add(time.Duration(randInt(-6, 10)) * time.Hour)
		actualArrival = &a
	case "delayed":
		departure = now.AddDate(0, 0, -randInt(1, 10))
		estimatedArrival = departure.AddDate(0, 0, randInt(1, 4))
	case "in_transit":
		departure = now.AddDate(0, 0, -randInt(0, 3))
		estimatedArrival = now.AddDate(0, 0, randInt(1, 5))
	case "scheduled":
		departure = now.AddDate(0, 0, randInt(1, 25))
		estimatedArrival = departure.AddDate(0, 0, randInt(1, 5))
	case "cancelled":
		departure = now.AddDate(0, 0, randInt(-20, 20))
		estimatedArrival = departure.AddDate(0, 0, 2)
	default:
		departure = now
		estimatedArrival = now.AddDate(0, 0, 2)
	}
	return
}

func freightFor(vehicleType string) float64 {
	switch vehicleType {
	case "ship":
		return randFloat(20000, 60000)
	case "plane":
		return randFloat(8000, 25000)
	default:
		return randFloat(800, 4000)
	}
}

func randChoice[T any](items []T) T {
	return items[rand.Intn(len(items))]
}

func randFloat(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

func randInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func randomHex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}
