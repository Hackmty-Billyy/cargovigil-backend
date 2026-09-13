DROP TABLE IF EXISTS trip_contingency_funds;
DROP TABLE IF EXISTS company_cost_settings;

ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_category_check;
ALTER TABLE expenses ADD CONSTRAINT expenses_category_check
    CHECK (category IN ('fuel', 'toll', 'maintenance', 'driver_payroll', 'port_fees', 'insurance', 'other'));

ALTER TABLE trip_frictions DROP COLUMN IF EXISTS opportunity_cost;
