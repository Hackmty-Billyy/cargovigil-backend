-- Módulo 3 — Gestión y Cobertura de Insumos / Combustible.
-- fuel_indexes and trip_fuel_logs already exist (000014). What's missing per
-- docs/project.txt is FuelSurchargeRule and RouteMarginImpact.

-- FuelSurchargeRule: per-company rule that compares the latest fuel_indexes
-- price for a (fuel_type, region) against the baseline price used when
-- routes/contracts were priced, and recommends a surcharge percentage when
-- the increase exceeds an absorbable threshold. Recalculated by
-- jobs.StartFuelSurchargeRecalcJob (mirrors route_risk_profiles/treasury's
-- daily forecast: a persisted, periodically-refreshed snapshot rather than a
-- pure request-time calculation).
CREATE TABLE fuel_surcharge_rules (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    fuel_type                       VARCHAR(30) NOT NULL CHECK (fuel_type IN ('diesel', 'bunker_c', 'marine_gasoil', 'jet_a1')),
    region                          VARCHAR(100) NOT NULL,
    baseline_price                  NUMERIC(10, 4) NOT NULL,
    threshold_percentage            NUMERIC(5, 2) NOT NULL DEFAULT 5.00,
    pass_through_rate               NUMERIC(5, 2) NOT NULL DEFAULT 100.00,
    last_index_price                NUMERIC(10, 4),
    price_variation_percentage      NUMERIC(6, 2) NOT NULL DEFAULT 0.00,
    suggested_surcharge_percentage  NUMERIC(6, 2) NOT NULL DEFAULT 0.00,
    is_active                       BOOLEAN NOT NULL DEFAULT true,
    last_calculated_at              TIMESTAMPTZ,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_fuel_surcharge_rules_scope ON fuel_surcharge_rules (company_id, fuel_type, region);
CREATE INDEX idx_fuel_surcharge_rules_company_id ON fuel_surcharge_rules (company_id);

-- RouteMarginImpact: latest "what-if" simulation snapshot per (route,
-- fuel_type) — how a hypothetical % change in fuel price would erode the
-- margin on that route, based on that route's own historical fuel spend
-- (trip_fuel_logs) and freight revenue (trips.agreed_freight_price). One row
-- per scope, upserted every time POST .../simulate runs, same shape as
-- route_risk_profiles.
CREATE TABLE route_margin_impacts (
    id                                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                             UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    route_id                               UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    fuel_type                              VARCHAR(30) NOT NULL CHECK (fuel_type IN ('diesel', 'bunker_c', 'marine_gasoil', 'jet_a1')),
    baseline_avg_fuel_cost                 NUMERIC(14, 2) NOT NULL,
    baseline_avg_revenue                   NUMERIC(14, 2) NOT NULL,
    simulated_price_variation_percentage   NUMERIC(6, 2) NOT NULL,
    simulated_fuel_cost                    NUMERIC(14, 2) NOT NULL,
    cost_increase                          NUMERIC(14, 2) NOT NULL,
    margin_impact_percentage               NUMERIC(6, 2) NOT NULL,
    sample_trip_count                      INT NOT NULL DEFAULT 0,
    calculated_at                          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_route_margin_impacts_scope ON route_margin_impacts (company_id, route_id, fuel_type);
CREATE INDEX idx_route_margin_impacts_company_id ON route_margin_impacts (company_id);
