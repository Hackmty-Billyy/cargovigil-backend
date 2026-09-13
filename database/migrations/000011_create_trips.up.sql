CREATE TABLE trips (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id             UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vehicle_id             UUID NOT NULL REFERENCES vehicles(id) ON DELETE RESTRICT,
    route_id               UUID NOT NULL REFERENCES routes(id) ON DELETE RESTRICT,
    client_id              UUID NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    contract_id            UUID REFERENCES contracts(id) ON DELETE SET NULL,
    tracking_code          VARCHAR(50) NOT NULL,
    cargo_type             VARCHAR(100) NOT NULL DEFAULT 'general',
    cargo_weight_tons      NUMERIC(10, 2) NOT NULL DEFAULT 0.00,
    status                 VARCHAR(30) NOT NULL CHECK (status IN ('scheduled', 'in_transit', 'delayed', 'completed', 'cancelled')) DEFAULT 'scheduled',
    departure_date         TIMESTAMPTZ NOT NULL,
    estimated_arrival_date TIMESTAMPTZ NOT NULL,
    actual_arrival_date    TIMESTAMPTZ,
    agreed_freight_price   NUMERIC(14, 2) NOT NULL,
    currency               VARCHAR(3) NOT NULL DEFAULT 'USD',
    fuel_surcharge_amount  NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    contingency_budget     NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_trips_company_tracking ON trips (company_id, tracking_code);
CREATE INDEX idx_trips_company_id ON trips (company_id);
CREATE INDEX idx_trips_vehicle_id ON trips (vehicle_id);
CREATE INDEX idx_trips_route_id ON trips (route_id);
CREATE INDEX idx_trips_client_id ON trips (client_id);
CREATE INDEX idx_trips_status ON trips (status);
CREATE INDEX idx_trips_departure_date ON trips (departure_date);
