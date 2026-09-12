ALTER TABLE users ADD COLUMN company_id UUID REFERENCES companies(id);

-- Backfill: any user that predates tenancy (e.g. the dev seed users from
-- migration 000005) gets parked in a placeholder company so the column can
-- be made NOT NULL below.
DO $$
DECLARE
    dev_company_id UUID;
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE company_id IS NULL) THEN
        INSERT INTO companies (name) VALUES ('CargoVigil Dev') RETURNING id INTO dev_company_id;
        UPDATE users SET company_id = dev_company_id WHERE company_id IS NULL;
    END IF;
END $$;

ALTER TABLE users ALTER COLUMN company_id SET NOT NULL;
CREATE INDEX idx_users_company_id ON users (company_id);
