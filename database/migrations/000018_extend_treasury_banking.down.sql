DROP TABLE IF EXISTS cash_flow_projections;

ALTER TABLE expenses DROP COLUMN IF EXISTS bank_account_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS bank_account_id;
