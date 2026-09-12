-- DEV ONLY: usuarios de prueba para calar el flujo de auth localmente.
-- Password para los 4: Password123!
-- No correr esta migracion contra una base de datos real/produccion.

INSERT INTO users (email, password_hash, role_id, is_active) VALUES
('admin@cargovigil.test',
 '$argon2id$v=19$m=19456,t=2,p=1$p9Q4kZYGgR/cvFSZjQI/fQ$6UucGQ5JgDMoY0skPG/x65fagHeTxvcZpIl9sePy0xM',
 (SELECT id FROM roles WHERE name = 'admin'), true),
('operaciones@cargovigil.test',
 '$argon2id$v=19$m=19456,t=2,p=1$5ovBmICz4PxiGLqoBfNwng$RFETLeCaIkK3+IuNssEIYt/RyMPG+1vbmQ1L8s+PawY',
 (SELECT id FROM roles WHERE name = 'operations'), true),
('finanzas@cargovigil.test',
 '$argon2id$v=19$m=19456,t=2,p=1$t3xpsodXXPwdi7qsbFy0Ow$ZQWL6xbtvqMPRi9x/TyRm2Srn6w+niH9zKj5xKLw8ZQ',
 (SELECT id FROM roles WHERE name = 'finance'), true);

-- Este usuario ya tiene TOTP habilitado y confirmado, para probar el flujo
-- completo de 2FA sin pasar por /auth/totp/enroll primero. El secreto en
-- claro es IMYFY3YGCSANU4JLTM7X22UGBBSEC63L (agregalo a Google Authenticator
-- o cualquier app TOTP para generar codigos validos).
INSERT INTO users (email, password_hash, role_id, is_active, totp_enabled, totp_secret_enc, totp_confirmed_at) VALUES
('admin.mfa@cargovigil.test',
 '$argon2id$v=19$m=19456,t=2,p=1$EDg10piK1+nTmkpd18BxbQ$HT13/H3yqBfn/XaWiv7202iudwBbn8JWwDDDjLQazrg',
 (SELECT id FROM roles WHERE name = 'admin'), true, true,
 'lYvbWP4MCJo94lo77TewWFmXV2n2mYcwaU9oMK7mi+J1nGc6e5whe44JwjQuodIXwyfzePJoX2TQRlRZ',
 now());

INSERT INTO user_recovery_codes (user_id, code_hash)
SELECT u.id, h.code_hash
FROM users u
CROSS JOIN (VALUES
    ('ca184d685d3997e4f9a171907148d7f7f190872e34bffc0057b49e7e61336943'),
    ('19ab655a45bba5902999042e014e30936bc9fa00c46b3c11567bb2bd3529f2b9'),
    ('d3cd25b48a6bf7c24b9891da9061b1fa3ea2910e9b634276e119b6e4ee2704be'),
    ('3c79214cab564fcbaf92e7a213b3dbc603e052d51c6281ad18966fd2ecf10c74'),
    ('884c372915a329e6f4515f4cfeb1a3b240c4397403b1b8ce6b14e71d57324221'),
    ('ee89e084d6b8311bc043fd49873c48779cda7446f57a1c770fe9e0f934ef46fb'),
    ('b3c674b1c6635588c1e325c2bc70f034c7b090c085e3c1fa8f4a14412169ca80'),
    ('2cea76549d78562d20d57b7f98eaaf52cdb7a679ae1dca2d2efbc2d402efac1d'),
    ('bbf00c9a3bf96513e484c3a49e16766c6bef6d237f58bccd518789c947da04d4'),
    ('a2e2a5bd1540a90f9166d5da6446f3f90e1e1d3e5d8ad69cd39c6098c46d4b5c')
) AS h(code_hash)
WHERE u.email = 'admin.mfa@cargovigil.test';
