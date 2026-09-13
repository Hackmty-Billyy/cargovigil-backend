-- Live tracking / radar layer.
--
-- The dashboard's live radar needs two things the schema never had: where a
-- trip physically is right now, and where its route starts and ends. Nothing
-- integrates a real GPS feed yet (same stopgap as treasury's bank feed and
-- fuel's price provider), so these columns are fed by the simulation endpoint
-- POST /logistics/trips/advance-simulation, which stands in for what a
-- telemetry sync would push automatically.

ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS current_lat          NUMERIC(9, 6),
    ADD COLUMN IF NOT EXISTS current_lng          NUMERIC(9, 6),
    ADD COLUMN IF NOT EXISTS progress_percentage  NUMERIC(5, 2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS is_stuck             BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS stuck_reason         TEXT,
    ADD COLUMN IF NOT EXISTS estimated_fuel_cost  NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS estimated_loss_risk  NUMERIC(14, 2) NOT NULL DEFAULT 0.00,
    ADD COLUMN IF NOT EXISTS last_simulated_step  INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_trips_is_stuck ON trips (is_stuck) WHERE is_stuck = true;

ALTER TABLE routes
    ADD COLUMN IF NOT EXISTS origin_lat NUMERIC(9, 6),
    ADD COLUMN IF NOT EXISTS origin_lng NUMERIC(9, 6),
    ADD COLUMN IF NOT EXISTS dest_lat   NUMERIC(9, 6),
    ADD COLUMN IF NOT EXISTS dest_lng   NUMERIC(9, 6);

-- Coordinates for the places the seeded routes actually use, so the map opens
-- with real positions instead of every vehicle stacked on a fallback point.
-- Routes created later get their coordinates from whoever registers them;
-- a route without coordinates simply doesn't draw a line.
WITH places (keyword, lat, lng) AS (
    VALUES ('%monterrey%',      25.686600, -100.316100),
           ('%laredo%',         27.476100,  -99.516400),
           ('%guadalajara%',    20.659700, -103.349600),
           ('%manzanillo%',     19.052200, -104.315800),
           ('%veracruz%',       19.190300,  -96.153300),
           ('%houston%',        29.735500,  -95.274700),
           ('%cdmx%',           19.436100,  -99.071900),
           ('%ciudad de m_xico%', 19.436100, -99.071900),
           ('%miami%',          25.795900,  -80.287100),
           ('%altamira%',       22.466700,  -97.933300),
           ('%l_zaro c_rdenas%', 17.955600, -102.191700),
           ('%tijuana%',        32.514900, -117.038200),
           ('%canc_n%',         21.161900,  -86.851500)
)
UPDATE routes r
   SET origin_lat = p.lat, origin_lng = p.lng
  FROM places p
 WHERE r.origin_lat IS NULL AND r.origin ILIKE p.keyword;

WITH places (keyword, lat, lng) AS (
    VALUES ('%monterrey%',      25.686600, -100.316100),
           ('%laredo%',         27.476100,  -99.516400),
           ('%guadalajara%',    20.659700, -103.349600),
           ('%manzanillo%',     19.052200, -104.315800),
           ('%veracruz%',       19.190300,  -96.153300),
           ('%houston%',        29.735500,  -95.274700),
           ('%cdmx%',           19.436100,  -99.071900),
           ('%ciudad de m_xico%', 19.436100, -99.071900),
           ('%miami%',          25.795900,  -80.287100),
           ('%altamira%',       22.466700,  -97.933300),
           ('%l_zaro c_rdenas%', 17.955600, -102.191700),
           ('%tijuana%',        32.514900, -117.038200),
           ('%canc_n%',         21.161900,  -86.851500)
)
UPDATE routes r
   SET dest_lat = p.lat, dest_lng = p.lng
  FROM places p
 WHERE r.dest_lat IS NULL AND r.destination ILIKE p.keyword;

-- Trips already in flight get placed at their route's origin so the first
-- render of the radar isn't empty.
UPDATE trips t
   SET current_lat = r.origin_lat, current_lng = r.origin_lng
  FROM routes r
 WHERE r.id = t.route_id
   AND t.current_lat IS NULL
   AND r.origin_lat IS NOT NULL
   AND t.status IN ('scheduled', 'in_transit', 'delayed');
