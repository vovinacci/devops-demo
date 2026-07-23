-- Dedicated read-only role for Grafana's Postgres datasource
-- (observability/grafana/provisioning/datasources/datasource.yml):
-- least privilege for a dashboard consumer -- CONNECT + schema USAGE +
-- SELECT on exactly the tables the Analytics History dashboard reads,
-- instead of handing Grafana the full analytics account. Demo-grade
-- credential, same posture as every other checked-in default in this
-- repo. New dashboard tables need an explicit new GRANT here (visible,
-- deliberate -- no blanket default privileges).
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'grafana_ro') THEN
        CREATE ROLE grafana_ro LOGIN PASSWORD 'grafana_ro';
    END IF;
    -- current_database(), not a hardcoded name: the same migration runs
    -- against the compose database (analytics) and the test database
    -- (analytics_test); GRANT takes no function, hence EXECUTE.
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO grafana_ro', current_database());
END
$$;

GRANT USAGE ON SCHEMA public TO grafana_ro;
GRANT SELECT ON event_buckets, current_items, seed_marker TO grafana_ro;
