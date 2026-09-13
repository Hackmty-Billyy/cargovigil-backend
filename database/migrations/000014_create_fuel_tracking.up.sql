CREATE TABLE fuel_indexes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fuel_type       VARCHAR(30) NOT NULL CHECK (fuel_type IN ('diesel', 'bunker_c', 'marine_gasoil', 'jet_a1')),
    region          VARCHAR(100) NOT NULL,
    price_per_unit  NUMERIC(10, 4) NOT NULL,
    unit_of_measure VARCHAR(20) NOT NULL DEFAULT 'liter',
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    source          VARCHAR(100) NOT NULL DEFAULT 'Global Fuel Index'
);
CREATE INDEX idx_fuel_indexes_type_region ON fuel_indexes (fuel_type, region);
CREATE INDEX idx_fuel_indexes_recorded_at ON fuel_indexes (recorded_at);

CREATE TABLE trip_fuel_logs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id           UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    fuel_type         VARCHAR(30) NOT NULL,
    volume_purchased  NUMERIC(10, 2) NOT NULL,
    cost_per_unit     NUMERIC(10, 4) NOT NULL,
    total_cost        NUMERIC(14, 2) NOT NULL,
    odometer_or_hours NUMERIC(10, 2),
    purchased_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trip_fuel_logs_company_id ON trip_fuel_logs (company_id);
CREATE INDEX idx_trip_fuel_logs_trip_id ON trip_fuel_logs (trip_id);
