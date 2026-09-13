-- The simulation advances in fixed steps of simulated hours, so an incident
-- that spans several steps has to know at which step it began — otherwise the
-- friction it opens in module 2 closes with a wall-clock duration of a few
-- seconds and reports zero idle cost, which is exactly the number the whole
-- module exists to produce.
ALTER TABLE trips ADD COLUMN IF NOT EXISTS stuck_since_step INT NOT NULL DEFAULT 0;
