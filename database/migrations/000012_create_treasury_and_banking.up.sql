CREATE TABLE bank_accounts (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    bank_name                VARCHAR(100) NOT NULL,
    account_number_mask      VARCHAR(20) NOT NULL,
    currency                 VARCHAR(3) NOT NULL DEFAULT 'USD',
    current_balance          NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    minimum_required_balance NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    is_active                BOOLEAN NOT NULL DEFAULT true,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_bank_accounts_company_id ON bank_accounts (company_id);

CREATE TABLE invoices (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id         UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id            UUID REFERENCES trips(id) ON DELETE SET NULL,
    client_id          UUID NOT NULL REFERENCES clients(id) ON DELETE RESTRICT,
    invoice_number     VARCHAR(100) NOT NULL,
    issue_date         DATE NOT NULL,
    due_date           DATE NOT NULL,
    adjusted_due_date  DATE NOT NULL,
    total_amount       NUMERIC(14, 2) NOT NULL,
    paid_amount        NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    status             VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'issued', 'partially_paid', 'paid', 'overdue', 'cancelled')) DEFAULT 'issued',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_invoices_company_number ON invoices (company_id, invoice_number);
CREATE INDEX idx_invoices_company_id ON invoices (company_id);
CREATE INDEX idx_invoices_trip_id ON invoices (trip_id);
CREATE INDEX idx_invoices_client_id ON invoices (client_id);
CREATE INDEX idx_invoices_status ON invoices (status);
CREATE INDEX idx_invoices_due_date ON invoices (due_date);
CREATE INDEX idx_invoices_adjusted_due_date ON invoices (adjusted_due_date);

CREATE TABLE expenses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    trip_id     UUID REFERENCES trips(id) ON DELETE SET NULL,
    category    VARCHAR(50) NOT NULL CHECK (category IN ('fuel', 'toll', 'maintenance', 'driver_payroll', 'port_fees', 'insurance', 'other')),
    description VARCHAR(255) NOT NULL,
    amount      NUMERIC(14, 2) NOT NULL,
    due_date    DATE NOT NULL,
    paid_date   DATE,
    status      VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'paid', 'cancelled')) DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_expenses_company_id ON expenses (company_id);
CREATE INDEX idx_expenses_trip_id ON expenses (trip_id);
CREATE INDEX idx_expenses_category ON expenses (category);
CREATE INDEX idx_expenses_due_date ON expenses (due_date);
CREATE INDEX idx_expenses_status ON expenses (status);

CREATE TABLE cash_alerts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    projected_date    DATE NOT NULL,
    severity          VARCHAR(20) NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    projected_deficit NUMERIC(14, 2) NOT NULL,
    description       TEXT NOT NULL,
    is_resolved       BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_cash_alerts_company_id ON cash_alerts (company_id);
CREATE INDEX idx_cash_alerts_projected_date ON cash_alerts (projected_date);
