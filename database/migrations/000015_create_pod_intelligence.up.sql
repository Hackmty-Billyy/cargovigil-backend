CREATE TABLE pod_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id         UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    client_id       UUID NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    document_url    TEXT,
    status          VARCHAR(30) NOT NULL CHECK (status IN ('pending_upload', 'uploaded_pending_review', 'under_dispute', 'approved_by_client', 'rejected')) DEFAULT 'pending_upload',
    dispute_reason  TEXT,
    delivery_date   TIMESTAMPTZ,
    signature_date  TIMESTAMPTZ,
    days_to_sign    INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pod_documents_company_id ON pod_documents (company_id);
CREATE INDEX idx_pod_documents_trip_id ON pod_documents (trip_id);
CREATE INDEX idx_pod_documents_client_id ON pod_documents (client_id);
CREATE INDEX idx_pod_documents_status ON pod_documents (status);

CREATE TABLE client_payment_behavior (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    client_id                   UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    average_pod_approval_days   NUMERIC(5, 1) NOT NULL DEFAULT 3.0,
    average_payment_delay_days  NUMERIC(5, 1) NOT NULL DEFAULT 0.0,
    dispute_rate_percentage     NUMERIC(5, 2) NOT NULL DEFAULT 0.00,
    last_calculated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_client_behavior_company_client ON client_payment_behavior (company_id, client_id);
CREATE INDEX idx_client_behavior_company_id ON client_payment_behavior (company_id);
