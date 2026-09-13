-- Best-effort rollback only. TimescaleDB does not offer a supported way to
-- turn a hypertable back into a plain table, so this removes the policies
-- and continuous aggregates but leaves the four tables as hypertables. If
-- this ever genuinely needs to be undone, restore from a backup taken
-- before migration 000019 instead.

SELECT remove_compression_policy('trip_fuel_logs', if_exists => true);
ALTER TABLE trip_fuel_logs SET (timescaledb.compress = false);

SELECT remove_continuous_aggregate_policy('route_friction_weekly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS route_friction_weekly;

SELECT remove_continuous_aggregate_policy('fuel_price_weekly', if_exists => true);
DROP MATERIALIZED VIEW IF EXISTS fuel_price_weekly;

SELECT remove_retention_policy('cash_flow_projections', if_exists => true);
