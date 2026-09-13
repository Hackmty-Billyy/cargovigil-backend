-- DEV / DEMO SEED: Datos de prueba para simular la operación logística multimodal
-- y alimentar los motores de tesorería, combustible, fricciones, POD y rentabilidad.

DO $$
DECLARE
    dev_comp_id UUID;
    v_truck1 UUID;
    v_truck2 UUID;
    v_ship UUID;
    v_plane UUID;
    r_mty_laredo UUID;
    r_ver_houston UUID;
    r_cdmx_mia UUID;
    r_gdl_mzanillo UUID;
    c_ternium UUID;
    c_femsa UUID;
    c_maersk UUID;
    c_dhl UUID;
    ct_ternium UUID;
    ct_femsa UUID;
    ct_maersk UUID;
    t_trip1 UUID;
    t_trip2 UUID;
    t_trip3 UUID;
    t_trip4 UUID;
BEGIN
    SELECT id INTO dev_comp_id FROM companies WHERE name = 'CargoVigil Dev' LIMIT 1;
    IF dev_comp_id IS NULL THEN
        SELECT id INTO dev_comp_id FROM companies ORDER BY created_at ASC LIMIT 1;
    END IF;

    IF dev_comp_id IS NOT NULL THEN
        -- 1. VEHICULOS
        INSERT INTO vehicles (company_id, type, identifier, is_active)
        VALUES (dev_comp_id, 'truck', 'T-KW-8841 (Kenworth T680)', true) RETURNING id INTO v_truck1;

        INSERT INTO vehicles (company_id, type, identifier, is_active)
        VALUES (dev_comp_id, 'truck', 'T-FL-2049 (Freightliner Cascadia)', true) RETURNING id INTO v_truck2;

        INSERT INTO vehicles (company_id, type, identifier, is_active)
        VALUES (dev_comp_id, 'ship', 'S-MSK-9901 (Maersk Sealand Voyager)', true) RETURNING id INTO v_ship;

        INSERT INTO vehicles (company_id, type, identifier, is_active)
        VALUES (dev_comp_id, 'plane', 'A-B767-4402 (Boeing 767-300F)', true) RETURNING id INTO v_plane;

        -- 2. RUTAS
        INSERT INTO routes (company_id, origin, destination, distance_km, is_active)
        VALUES (dev_comp_id, 'Monterrey, NL', 'Nuevo Laredo, TAMPS', 220.00, true) RETURNING id INTO r_mty_laredo;

        INSERT INTO routes (company_id, origin, destination, distance_km, is_active)
        VALUES (dev_comp_id, 'Puerto de Veracruz, VER', 'Port of Houston, TX', 1380.00, true) RETURNING id INTO r_ver_houston;

        INSERT INTO routes (company_id, origin, destination, distance_km, is_active)
        VALUES (dev_comp_id, 'Aeropuerto AICM, CDMX', 'Miami Int. Airport, MIA', 2050.00, true) RETURNING id INTO r_cdmx_mia;

        INSERT INTO routes (company_id, origin, destination, distance_km, is_active)
        VALUES (dev_comp_id, 'Guadalajara, JAL', 'Puerto de Manzanillo, COL', 295.00, true) RETURNING id INTO r_gdl_mzanillo;

        -- 3. CLIENTES
        INSERT INTO clients (company_id, name, tax_id, is_active)
        VALUES (dev_comp_id, 'Ternium México S.A. de C.V.', 'TME901201AA1', true) RETURNING id INTO c_ternium;

        INSERT INTO clients (company_id, name, tax_id, is_active)
        VALUES (dev_comp_id, 'Femsa Logística S.A. de C.V.', 'FLE040518BB2', true) RETURNING id INTO c_femsa;

        INSERT INTO clients (company_id, name, tax_id, is_active)
        VALUES (dev_comp_id, 'Maersk Logistics & Services', 'MLS110822CC3', true) RETURNING id INTO c_maersk;

        INSERT INTO clients (company_id, name, tax_id, is_active)
        VALUES (dev_comp_id, 'DHL Global Forwarding', 'DGF980315DD4', true) RETURNING id INTO c_dhl;

        -- 4. CONTRATOS
        INSERT INTO contracts (company_id, client_id, reference, starts_on, ends_on, is_active)
        VALUES (dev_comp_id, c_ternium, 'CTR-TERNIUM-2026-A', '2026-01-01', '2026-12-31', true) RETURNING id INTO ct_ternium;

        INSERT INTO contracts (company_id, client_id, reference, starts_on, ends_on, is_active)
        VALUES (dev_comp_id, c_femsa, 'CTR-FEMSA-2026-Q1', '2026-01-01', '2026-06-30', true) RETURNING id INTO ct_femsa;

        INSERT INTO contracts (company_id, client_id, reference, starts_on, ends_on, is_active)
        VALUES (dev_comp_id, c_maersk, 'CTR-MAERSK-GULF-2026', '2026-01-01', '2026-12-31', true) RETURNING id INTO ct_maersk;

        -- 5. CUENTAS BANCARIAS
        INSERT INTO bank_accounts (company_id, bank_name, account_number_mask, currency, current_balance, minimum_required_balance)
        VALUES
        (dev_comp_id, 'BBVA Bancomer Operaciones', '**** 4892', 'MXN', 1250000.00, 300000.00),
        (dev_comp_id, 'Banorte Tesorería USD', '**** 9102', 'USD', 85000.00, 20000.00);

        -- 6. INDICES DE COMBUSTIBLE
        INSERT INTO fuel_indexes (fuel_type, region, price_per_unit, unit_of_measure, source)
        VALUES
        ('diesel', 'México - Noreste', 24.8500, 'liter', 'Comisión Reguladora de Energía'),
        ('bunker_c', 'US Gulf Coast', 620.5000, 'metric_ton', 'Platts Bunkerwire'),
        ('marine_gasoil', 'Port of Houston', 785.0000, 'metric_ton', 'Bunker Index'),
        ('jet_a1', 'México - AICM', 18.4000, 'liter', 'ASA Combustibles');

        -- 7. PERFILES DE RIESGO POR RUTA
        INSERT INTO route_risk_profiles (company_id, route_id, historical_risk_score, avg_delay_hours, suggested_contingency_percentage, incident_count)
        VALUES
        (dev_comp_id, r_mty_laredo, 1.45, 4.20, 8.50, 12),
        (dev_comp_id, r_ver_houston, 1.15, 8.50, 5.00, 3),
        (dev_comp_id, r_cdmx_mia, 1.05, 1.20, 3.00, 1),
        (dev_comp_id, r_gdl_mzanillo, 1.30, 3.80, 7.00, 8);

        -- 8. COMPORTAMIENTO DE PAGO DE CLIENTES
        INSERT INTO client_payment_behavior (company_id, client_id, average_pod_approval_days, average_payment_delay_days, dispute_rate_percentage)
        VALUES
        (dev_comp_id, c_ternium, 2.5, 0.5, 1.20),
        (dev_comp_id, c_femsa, 6.2, 5.0, 7.50),
        (dev_comp_id, c_maersk, 3.0, 1.0, 2.00),
        (dev_comp_id, c_dhl, 2.0, 0.0, 0.50);

        -- 9. VIAJES MULTIMODALES (TRIPS)
        -- Viaje 1: Terrestre completado (Monterrey -> Laredo)
        INSERT INTO trips (company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type, cargo_weight_tons, status, departure_date, estimated_arrival_date, actual_arrival_date, agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget)
        VALUES (dev_comp_id, v_truck1, r_mty_laredo, c_ternium, ct_ternium, 'TRIP-2026-001', 'Bobinas de Acero', 28.50, 'completed', now() - interval '3 days', now() - interval '2 days 18 hours', now() - interval '2 days 15 hours', 1850.00, 'USD', 120.00, 157.25)
        RETURNING id INTO t_trip1;

        -- Viaje 2: Marítimo en tránsito (Veracruz -> Houston)
        INSERT INTO trips (company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type, cargo_weight_tons, status, departure_date, estimated_arrival_date, agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget)
        VALUES (dev_comp_id, v_ship, r_ver_houston, c_maersk, ct_maersk, 'TRIP-2026-002', 'Contenedores Químicos', 4500.00, 'in_transit', now() - interval '1 day', now() + interval '2 days', 48000.00, 'USD', 3500.00, 2400.00)
        RETURNING id INTO t_trip2;

        -- Viaje 3: Aéreo programado (CDMX -> Miami)
        INSERT INTO trips (company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type, cargo_weight_tons, status, departure_date, estimated_arrival_date, agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget)
        VALUES (dev_comp_id, v_plane, r_cdmx_mia, c_dhl, NULL, 'TRIP-2026-003', 'Carga Farmacéutica Perecedera', 12.00, 'scheduled', now() + interval '1 day', now() + interval '1 day 5 hours', 14500.00, 'USD', 950.00, 435.00)
        RETURNING id INTO t_trip3;

        -- Viaje 4: Terrestre retrasado (Guadalajara -> Manzanillo)
        INSERT INTO trips (company_id, vehicle_id, route_id, client_id, contract_id, tracking_code, cargo_type, cargo_weight_tons, status, departure_date, estimated_arrival_date, agreed_freight_price, currency, fuel_surcharge_amount, contingency_budget)
        VALUES (dev_comp_id, v_truck2, r_gdl_mzanillo, c_femsa, ct_femsa, 'TRIP-2026-004', 'Bebidas Embotelladas', 26.00, 'delayed', now() - interval '18 hours', now() - interval '8 hours', 2200.00, 'USD', 140.00, 154.00)
        RETURNING id INTO t_trip4;

        -- 10. FRICCIONES / TIEMPOS MUERTOS
        INSERT INTO trip_frictions (company_id, trip_id, event_type, location_name, started_at, ended_at, duration_hours, cost_impact, notes)
        VALUES
        (dev_comp_id, t_trip1, 'customs_delay', 'Puente Internacional Comercio Mundial - Laredo', now() - interval '2 days 20 hours', now() - interval '2 days 16 hours', 4.00, 180.00, 'Saturación en rayos gamma aduana americana'),
        (dev_comp_id, t_trip4, 'traffic_congestion', 'Autopista Colima-Manzanillo Km 72', now() - interval '14 hours', now() - interval '9 hours', 5.00, 225.00, 'Bloqueo por derrumbe en zona montañosa');

        -- 11. COMBUSTIBLE CONSUMIDO POR VIAJE
        INSERT INTO trip_fuel_logs (company_id, trip_id, fuel_type, volume_purchased, cost_per_unit, total_cost, odometer_or_hours)
        VALUES
        (dev_comp_id, t_trip1, 'diesel', 95.00, 1.25, 118.75, 220.00),
        (dev_comp_id, t_trip2, 'bunker_c', 35.00, 620.50, 21717.50, 1380.00),
        (dev_comp_id, t_trip4, 'diesel', 120.00, 1.25, 150.00, 295.00);

        -- 12. POD INTELLIGENCE & DOCUMENTACION
        INSERT INTO pod_documents (company_id, trip_id, client_id, document_url, status, delivery_date, signature_date, days_to_sign)
        VALUES
        (dev_comp_id, t_trip1, c_ternium, 'https://storage.cargovigil.test/pods/pod_trip_001_signed.pdf', 'approved_by_client', now() - interval '2 days 15 hours', now() - interval '2 days 12 hours', 1),
        (dev_comp_id, t_trip4, c_femsa, 'https://storage.cargovigil.test/pods/pod_trip_004_draft.pdf', 'under_dispute', NULL, NULL, 0);

        -- 13. FACTURAS / CUENTAS POR COBRAR
        INSERT INTO invoices (company_id, trip_id, client_id, invoice_number, issue_date, due_date, adjusted_due_date, total_amount, paid_amount, status)
        VALUES
        (dev_comp_id, t_trip1, c_ternium, 'FAC-2026-0089', CURRENT_DATE - 2, CURRENT_DATE + 28, CURRENT_DATE + 28, 1970.00, 1970.00, 'paid'),
        (dev_comp_id, t_trip2, c_maersk, 'FAC-2026-0090', CURRENT_DATE, CURRENT_DATE + 30, CURRENT_DATE + 32, 51500.00, 0.00, 'issued'),
        (dev_comp_id, t_trip4, c_femsa, 'FAC-2026-0091', CURRENT_DATE - 1, CURRENT_DATE + 15, CURRENT_DATE + 25, 2340.00, 0.00, 'issued');

        -- 14. GASTOS / CUENTAS POR PAGAR
        INSERT INTO expenses (company_id, trip_id, category, description, amount, due_date, status)
        VALUES
        (dev_comp_id, t_trip1, 'toll', 'Casetas Autopista Monterrey-Laredo', 65.00, CURRENT_DATE + 7, 'paid'),
        (dev_comp_id, t_trip1, 'driver_payroll', 'Viáticos y honorario operador camión #8841', 220.00, CURRENT_DATE + 5, 'paid'),
        (dev_comp_id, t_trip2, 'port_fees', 'Tarifas de atraque y práctico Puerto de Houston', 6800.00, CURRENT_DATE + 12, 'pending'),
        (dev_comp_id, t_trip4, 'maintenance', 'Revisión frenos y suspensión preventiva', 340.00, CURRENT_DATE + 10, 'pending');

        -- 15. ALERTAS DE TESORERIA PROYECTADAS
        INSERT INTO cash_alerts (company_id, projected_date, severity, projected_deficit, description, is_resolved)
        VALUES
        (dev_comp_id, CURRENT_DATE + 12, 'high', 4200.00, 'Déficit proyectado por coincidencia de pago de combustible marítimo y tarifas de atraque antes del cobro de FAC-2026-0090', false);

        -- 16. RENTABILIDAD CONSOLIDADA POR VIAJE (Módulo 5)
        INSERT INTO trip_profitability (company_id, trip_id, gross_revenue, fuel_cost, toll_and_port_cost, driver_cost, friction_cost, maintenance_allocation, total_cost, net_profit, profit_margin_percentage, profit_per_km)
        VALUES
        (dev_comp_id, t_trip1, 1970.00, 118.75, 65.00, 220.00, 180.00, 50.00, 633.75, 1336.25, 67.83, 6.07);

    END IF;
END $$;
