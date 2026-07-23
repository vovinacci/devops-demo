-- Flyway V1: report_jobs (RFC-0001 D2 Phase 6 PR-2).
--
-- One row per report job. The status column is the job state machine:
-- PENDING (accepted, queued) -> RUNNING (a worker picked it up) ->
-- SUCCEEDED (artifact written) | FAILED (error captured). Transitions are
-- persisted by the ReportService as the coroutine job advances, so a crash
-- leaves a job in its last durable state rather than losing it; any job left
-- non-terminal (PENDING/RUNNING) is reconciled to FAILED on the next startup
-- (ReportService.reconcileOrphanedJobs -- single reports instance, so such a
-- row is by definition orphaned).
--
-- Generated artifacts (XLSX/PDF/CSV) live on the named artifact volume, never
-- in this table (RFC-0001 D2: BLOBs-in-Postgres deliberately avoided).
-- artifact_path is the path on that volume; artifact_bytes is its size, also
-- surfaced as a metric. params is the request payload (report-type inputs)
-- kept as jsonb for auditability and for the dashboard to slice on.
CREATE TABLE IF NOT EXISTS report_jobs (
    id             text PRIMARY KEY,
    type           text NOT NULL,
    format         text NOT NULL CHECK (format IN ('xlsx', 'pdf', 'csv')),
    status         text NOT NULL CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED')),
    params         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    started_at     timestamptz,
    finished_at    timestamptz,
    error          text,
    artifact_path  text,
    artifact_bytes bigint
);

-- Dashboard / operational queries slice by status and scan recent jobs in
-- creation order (e.g. "jobs by status", "most recent failures").
CREATE INDEX IF NOT EXISTS report_jobs_status_idx ON report_jobs (status);
CREATE INDEX IF NOT EXISTS report_jobs_created_at_idx ON report_jobs (created_at DESC);
