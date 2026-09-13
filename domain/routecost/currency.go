package routecost

import "math"

// TreasuryCurrency is the single currency the cash flow forecast of Module 1
// understands (its balances are summed MXN-only on purpose). Every amount this
// module pushes into treasury — reserves and friction costs — must land here.
const TreasuryCurrency = "MXN"

// FXRates holds the conversion rates this module is allowed to apply. Trips
// are priced in USD in practice while treasury reasons in MXN, so the reserve
// would be understated ~17x without an explicit conversion. The rate is
// configuration, not a market feed: it is snapshotted onto every fund row
// (ContingencyFund.FXRate) so a later rate change never rewrites history.
type FXRates struct {
	USDToMXN float64
}

// Convert returns amount expressed in `to`. Only the pairs this platform
// actually uses are supported; anything else is an explicit error rather than
// a silent 1:1 pass-through.
func (f FXRates) Convert(amount float64, from, to string) (float64, float64, error) {
	if from == to {
		return amount, 1, nil
	}
	switch {
	case from == "USD" && to == "MXN":
		return round2(amount * f.USDToMXN), f.USDToMXN, nil
	case from == "MXN" && to == "USD":
		if f.USDToMXN == 0 {
			return 0, 0, ErrUnsupportedCurrency
		}
		rate := 1 / f.USDToMXN
		return round2(amount * rate), rate, nil
	default:
		return 0, 0, ErrUnsupportedCurrency
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
