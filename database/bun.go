package database

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// NewBunDB wraps the app's existing *pgxpool.Pool as a *bun.DB, instead of
// opening a second connection pool. stdlib.OpenDBFromPool adapts pgxpool to
// database/sql (what Bun requires) while every query still goes through the
// same pooled connections as the rest of the app.
//
// IMPORTANT: Bun is used here purely as a query/mapping layer. Schema is
// owned exclusively by golang-migrate (RunMigrations, migrations/*.sql) —
// never call bun's AutoMigrate/schema sync against this DB. That split is
// what lets Bun coexist with the TimescaleDB-specific DDL already applied
// (hypertables with composite primary keys, continuous aggregates,
// compression/retention policies): Bun never sees or touches that DDL, it
// only SELECTs/INSERTs/UPDATEs/DELETEs against tables that already exist.
func NewBunDB(pool *pgxpool.Pool) *bun.DB {
	sqldb := stdlib.OpenDBFromPool(pool)
	return bun.NewDB(sqldb, pgdialect.New())
}
