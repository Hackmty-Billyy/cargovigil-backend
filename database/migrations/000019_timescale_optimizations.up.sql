-- TimescaleDB (Tiger Data) optimizations for the tables that are genuinely
-- time-series shaped. These are pure DB-level optimizations: no Go code
-- depends on the primary key shape changing here (verified against
-- domain/treasury, which only upserts/lists these rows, never looks them up
-- by their bare `id`).

-- 1. cash_flow_projections: hypertable + retention. The daily treasury job
--    rewrites this table's 60-day window every day; nothing today ever
--    prunes old rows, so it grows forever with projections nobody looks at
--    anymore. Chunked weekly, retained 90 days.
ALTER TABLE cash_flow_projections DROP CONSTRAINT cash_flow_projections_pkey;
ALTER TABLE cash_flow_projections ADD PRIMARY KEY (id, projected_date);
SELECT create_hypertable('cash_flow_projections', 'projected_date', migrate_data => true, chunk_time_interval => INTERVAL '7 days');
SELECT add_retention_policy('cash_flow_projections', INTERVAL '90 days');

-- 2. fuel_indexes: hypertable + weekly continuous aggregate per fuel_type/region,
--    so Module 3 can read a precomputed trend instead of scanning full history.
ALTER TABLE fuel_indexes DROP CONSTRAINT fuel_indexes_pkey;
ALTER TABLE fuel_indexes ADD PRIMARY KEY (id, recorded_at);
SELECT create_hypertable('fuel_indexes', 'recorded_at', migrate_data => true, chunk_time_interval => INTERVAL '30 days');

CREATE MATERIALIZED VIEW fuel_price_weekly
WITH (timescaledb.continuous) AS
SELECT
    fuel_type,
    region,
    time_bucket('7 days', recorded_at) AS bucket,
    avg(price_per_unit) AS avg_price,
    min(price_per_unit) AS min_price,
    max(price_per_unit) AS max_price,
    count(*) AS sample_count
FROM fuel_indexes
GROUP BY fuel_type, region, bucket
WITH NO DATA;

SELECT add_continuous_aggregate_policy('fuel_price_weekly',
    start_offset => INTERVAL '6 months',
    end_offset => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');

-- 3. trip_frictions: hypertable + weekly continuous aggregate per route. This
--    is an input Module 2 can query when it recalculates route_risk_profiles
--    (not built yet) — it does NOT write into route_risk_profiles itself.
ALTER TABLE trip_frictions DROP CONSTRAINT trip_frictions_pkey;
ALTER TABLE trip_frictions ADD PRIMARY KEY (id, started_at);
SELECT create_hypertable('trip_frictions', 'started_at', migrate_data => true, chunk_time_interval => INTERVAL '30 days');

CREATE MATERIALIZED VIEW route_friction_weekly
WITH (timescaledb.continuous) AS
SELECT
    tf.company_id,
    t.route_id,
    time_bucket('7 days', tf.started_at) AS bucket,
    count(*) AS incident_count,
    avg(tf.duration_hours) AS avg_duration_hours,
    sum(tf.cost_impact) AS total_cost_impact
FROM trip_frictions tf
JOIN trips t ON t.id = tf.trip_id
GROUP BY tf.company_id, t.route_id, bucket
WITH NO DATA;

SELECT add_continuous_aggregate_policy('route_friction_weekly',
    start_offset => INTERVAL '6 months',
    end_offset => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day');

-- 4. trip_fuel_logs: hypertable + native compression. Insert-only, never
--    updated once logged (no Go code updates it), so compressing anything
--    older than 30 days is safe and just saves disk.
ALTER TABLE trip_fuel_logs DROP CONSTRAINT trip_fuel_logs_pkey;
ALTER TABLE trip_fuel_logs ADD PRIMARY KEY (id, purchased_at);
SELECT create_hypertable('trip_fuel_logs', 'purchased_at', migrate_data => true, chunk_time_interval => INTERVAL '30 days');

ALTER TABLE trip_fuel_logs SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'company_id',
    timescaledb.compress_orderby = 'purchased_at DESC'
);
SELECT add_compression_policy('trip_fuel_logs', INTERVAL '30 days');
