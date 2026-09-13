-- Module 2 (operational cost & route friction intelligence).
-- trips, trip_frictions and route_risk_profiles already exist (000011 / 000013);
-- this migration only adds what the module needs on top of them.

-- Direct out-of-pocket cost (a demurrage invoice) already lives in
-- trip_frictions.cost_impact. Opportunity cost is a different number: the
-- revenue the asset failed to produce while it sat idle. It is persisted
-- instead of computed on read so that changing the hourly rate tomorrow does
-- not silently rewrite the history Module 5 reports on.
ALTER TABLE trip_frictions ADD COLUMN opportunity_cost NUMERIC(14, 2) NOT NULL DEFAULT 0.00;

-- Liquidity cushions reach the cash flow forecast as regular pending expenses
-- so Module 1 needs no changes, but they get their own category: a reserve is
-- not a real payable, and when the friction it was covering actually costs
-- money the reserve is consumed instead of stacked on top (no double counting).
ALTER TABLE expenses DROP CONSTRAINT expenses_category_check;
ALTER TABLE expenses ADD CONSTRAINT expenses_category_check
    CHECK (category IN ('fuel', 'toll', 'maintenance', 'driver_payroll', 'port_fees', 'insurance', 'contingency_reserve', 'other'));

-- Optional per-company override for the idle hourly rate. When absent, the
-- rate is derived from the trip itself (agreed_freight_price / planned hours).
CREATE TABLE company_cost_settings (
    company_id       UUID PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
    idle_hourly_rate NUMERIC(14, 2),
    currency         VARCHAR(3) NOT NULL DEFAULT 'MXN',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One cushion per trip. trips.contingency_budget stays as a denormalised
-- mirror of allocated_amount (in the trip's own currency) for the modules that
-- already read that column; this table is the auditable version: what was
-- reserved, on what basis, how much was actually consumed and what was
-- released when the trip closed.
CREATE TABLE trip_contingency_funds (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id            UUID NOT NULL UNIQUE REFERENCES trips(id) ON DELETE CASCADE,
    route_risk_score   NUMERIC(4, 2) NOT NULL DEFAULT 1.00,
    applied_percentage NUMERIC(5, 2) NOT NULL DEFAULT 5.00,
    base_amount        NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    base_currency      VARCHAR(3) NOT NULL DEFAULT 'MXN',
    fx_rate            NUMERIC(12, 6) NOT NULL DEFAULT 1.000000,
    reserve_currency   VARCHAR(3) NOT NULL DEFAULT 'MXN',
    allocated_amount   NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    consumed_amount    NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    released_amount    NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    status             VARCHAR(20) NOT NULL CHECK (status IN ('allocated', 'partially_consumed', 'exhausted', 'released')) DEFAULT 'allocated',
    reserve_expense_id UUID REFERENCES expenses(id) ON DELETE SET NULL,
    calculated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_trip_contingency_funds_company_id ON trip_contingency_funds (company_id);
CREATE INDEX idx_trip_contingency_funds_status ON trip_contingency_funds (status);
