DO $$
DECLARE
    dev_comp_id UUID;
BEGIN
    SELECT id INTO dev_comp_id FROM companies WHERE name = 'CargoVigil Dev' LIMIT 1;
    IF dev_comp_id IS NOT NULL THEN
        DELETE FROM trip_profitability WHERE company_id = dev_comp_id;
        DELETE FROM cash_alerts WHERE company_id = dev_comp_id;
        DELETE FROM expenses WHERE company_id = dev_comp_id;
        DELETE FROM invoices WHERE company_id = dev_comp_id;
        DELETE FROM pod_documents WHERE company_id = dev_comp_id;
        DELETE FROM trip_fuel_logs WHERE company_id = dev_comp_id;
        DELETE FROM trip_frictions WHERE company_id = dev_comp_id;
        DELETE FROM trips WHERE company_id = dev_comp_id;
        DELETE FROM client_payment_behavior WHERE company_id = dev_comp_id;
        DELETE FROM route_risk_profiles WHERE company_id = dev_comp_id;
        DELETE FROM fuel_indexes WHERE source IN ('Comisión Reguladora de Energía', 'Platts Bunkerwire', 'Bunker Index', 'ASA Combustibles');
        DELETE FROM bank_accounts WHERE company_id = dev_comp_id;
        DELETE FROM contracts WHERE company_id = dev_comp_id;
        DELETE FROM clients WHERE company_id = dev_comp_id;
        DELETE FROM routes WHERE company_id = dev_comp_id;
        DELETE FROM vehicles WHERE company_id = dev_comp_id;
    END IF;
END $$;
