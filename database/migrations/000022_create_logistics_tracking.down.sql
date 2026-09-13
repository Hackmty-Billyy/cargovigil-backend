DROP INDEX IF EXISTS idx_trips_is_stuck;

ALTER TABLE trips
    DROP COLUMN IF EXISTS current_lat,
    DROP COLUMN IF EXISTS current_lng,
    DROP COLUMN IF EXISTS progress_percentage,
    DROP COLUMN IF EXISTS is_stuck,
    DROP COLUMN IF EXISTS stuck_reason,
    DROP COLUMN IF EXISTS estimated_fuel_cost,
    DROP COLUMN IF EXISTS estimated_loss_risk,
    DROP COLUMN IF EXISTS last_simulated_step;

ALTER TABLE routes
    DROP COLUMN IF EXISTS origin_lat,
    DROP COLUMN IF EXISTS origin_lng,
    DROP COLUMN IF EXISTS dest_lat,
    DROP COLUMN IF EXISTS dest_lng;
