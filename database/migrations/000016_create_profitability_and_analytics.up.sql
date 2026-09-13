CREATE TABLE trip_profitability (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id                  UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    gross_revenue            NUMERIC(14, 2) NOT NULL,
    fuel_cost                NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    toll_and_port_cost       NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    driver_cost              NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    friction_cost            NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    maintenance_allocation   NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    total_cost               NUMERIC(14, 2) NOT NULL,
    net_profit               NUMERIC(14, 2) NOT NULL,
    profit_margin_percentage NUMERIC(6, 2) NOT NULL,
    profit_per_km            NUMERIC(10, 2) NOT NULL DEFAULT 0.00,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_trip_profitability_trip ON trip_profitability (trip_id);
CREATE INDEX idx_trip_profitability_company_id ON trip_profitability (company_id);
