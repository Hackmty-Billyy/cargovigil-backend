-- DEV ONLY: usuario de plataforma (staff de CargoVigil) para probar el alta
-- de empresas cliente. Password: Password123!
-- No correr esta migracion contra una base de datos real/produccion.

DO $$
DECLARE
    platform_company_id UUID;
BEGIN
    INSERT INTO companies (name) VALUES ('CargoVigil Platform') RETURNING id INTO platform_company_id;

    INSERT INTO users (company_id, email, password_hash, role_id, is_active)
    VALUES (
        platform_company_id,
        'platform@cargovigil.test',
        '$argon2id$v=19$m=19456,t=2,p=1$xJ18A1dREEydu+DudceiCQ$TDRNTm8gajhyZUPTvSb+SgwjkXrdpeAvXzeUJhoDWLo',
        (SELECT id FROM roles WHERE name = 'platform_admin'),
        true
    );
END $$;
