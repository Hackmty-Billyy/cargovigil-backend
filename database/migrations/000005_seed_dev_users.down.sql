DELETE FROM users WHERE email IN (
    'admin@cargovigil.test',
    'operaciones@cargovigil.test',
    'finanzas@cargovigil.test',
    'admin.mfa@cargovigil.test'
);
