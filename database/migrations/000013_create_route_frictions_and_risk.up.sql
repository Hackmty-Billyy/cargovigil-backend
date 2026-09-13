CREATE TABLE trip_frictions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id     UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id        UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    event_type     VARCHAR(50) NOT NULL CHECK (event_type IN ('port_demurrage', 'customs_delay', 'traffic_congestion', 'mechanical_failure', 'route_deviation', 'weather_hazard', 'security_incident', 'warehouse_detention')),
    location_name  VARCHAR(255),
    started_at     TIMESTAMPTZ NOT NULL,
    ended_at       TIMESTAMPTZ,
    duration_hours NUMERIC(6, 2) NOT NULL DEFAULT 0.00,
    cost_impact    NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trip_frictions_company_id ON trip_frictions (company_id);
CREATE INDEX idx_trip_frictions_trip_id ON trip_frictions (trip_id);
CREATE INDEX idx_trip_frictions_event_type ON trip_frictions (event_type);

CREATE TABLE route_risk_profiles (
    id                               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                       UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    route_id                         UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    historical_risk_score            NUMERIC(4, 2) NOT NULL DEFAULT 1.00,
    avg_delay_hours                  NUMERIC(6, 2) NOT NULL DEFAULT 0.00,
    suggested_contingency_percentage NUMERIC(5, 2) NOT NULL DEFAULT 5.00,
    incident_count                   INT NOT NULL DEFAULT 0,
    last_calculated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_route_risk_company_route ON route_risk_profiles (company_id, route_id);
CREATE INDEX idx_route_risk_company_id ON route_risk_profiles (company_id);
