ALTER TABLE invoices ADD COLUMN bank_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL;
ALTER TABLE expenses ADD COLUMN bank_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL;

CREATE INDEX idx_invoices_bank_account_id ON invoices (bank_account_id);
CREATE INDEX idx_expenses_bank_account_id ON expenses (bank_account_id);

CREATE TABLE cash_flow_projections (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    projected_date    DATE NOT NULL,
    projected_balance NUMERIC(14, 2) NOT NULL,
    generated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_cash_flow_projections_company_date ON cash_flow_projections (company_id, projected_date);
CREATE INDEX idx_cash_flow_projections_company_id ON cash_flow_projections (company_id);
