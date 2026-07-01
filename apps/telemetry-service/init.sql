CREATE TABLE IF NOT EXISTS telemetry (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    legacy_sensor_id TEXT NOT NULL,
    device_id       TEXT,
    metric          TEXT NOT NULL,
    value           DOUBLE PRECISION NOT NULL,
    unit            TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    observed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telemetry_legacy_sensor_id ON telemetry(legacy_sensor_id);
CREATE INDEX IF NOT EXISTS idx_telemetry_metric ON telemetry(metric);
CREATE INDEX IF NOT EXISTS idx_telemetry_observed_at ON telemetry(observed_at);
CREATE INDEX IF NOT EXISTS idx_telemetry_sensor_metric ON telemetry(legacy_sensor_id, metric);
