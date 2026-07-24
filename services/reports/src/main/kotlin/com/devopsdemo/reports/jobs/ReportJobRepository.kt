package com.devopsdemo.reports.jobs

import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportJob
import com.devopsdemo.reports.domain.ReportType
import com.fasterxml.jackson.databind.ObjectMapper
import org.springframework.jdbc.core.JdbcTemplate
import org.springframework.jdbc.core.RowMapper
import org.springframework.stereotype.Repository
import java.sql.ResultSet
import java.sql.Timestamp
import java.time.Instant

// Persistence for report_jobs. Each state transition is its own UPDATE so a
// job's durable state always reflects the last completed transition (a crash
// mid-render leaves it RUNNING, recoverable/visible, never lost).
@Repository
class ReportJobRepository(
    private val jdbc: JdbcTemplate,
    private val mapper: ObjectMapper,
) {
    fun insertPending(
        id: String,
        type: ReportType,
        format: ReportFormat,
        params: Map<String, Any?>,
    ) {
        jdbc.update(
            """
            INSERT INTO report_jobs (id, type, format, status, params)
            VALUES (?, ?, ?, ?, ?::jsonb)
            """.trimIndent(),
            id,
            type.wire,
            format.wire,
            JobStatus.PENDING.name,
            mapper.writeValueAsString(params),
        )
    }

    fun markRunning(
        id: String,
        startedAt: Instant,
    ) {
        jdbc.update(
            "UPDATE report_jobs SET status = ?, started_at = ? WHERE id = ?",
            JobStatus.RUNNING.name,
            Timestamp.from(startedAt),
            id,
        )
    }

    fun markSucceeded(
        id: String,
        artifactPath: String,
        artifactBytes: Long,
        finishedAt: Instant,
    ) {
        jdbc.update(
            """
            UPDATE report_jobs
            SET status = ?, artifact_path = ?, artifact_bytes = ?, finished_at = ?, error = NULL
            WHERE id = ?
            """.trimIndent(),
            JobStatus.SUCCEEDED.name,
            artifactPath,
            artifactBytes,
            Timestamp.from(finishedAt),
            id,
        )
    }

    fun markFailed(
        id: String,
        error: String,
        finishedAt: Instant,
    ) {
        jdbc.update(
            "UPDATE report_jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?",
            JobStatus.FAILED.name,
            error,
            Timestamp.from(finishedAt),
            id,
        )
    }

    // Startup reconciliation (single-instance deployment): any job still
    // PENDING or RUNNING at boot was orphaned by a crash or a shutdown that
    // interrupted an in-flight render, so drive it to a terminal FAILED state.
    // Returns the number of rows reconciled.
    fun failNonTerminal(
        error: String,
        finishedAt: Instant,
    ): Int =
        jdbc.update(
            """
            UPDATE report_jobs
            SET status = ?, error = ?, finished_at = ?
            WHERE status IN (?, ?)
            """.trimIndent(),
            JobStatus.FAILED.name,
            error,
            Timestamp.from(finishedAt),
            JobStatus.PENDING.name,
            JobStatus.RUNNING.name,
        )

    fun find(id: String): ReportJob? =
        jdbc
            .query("SELECT * FROM report_jobs WHERE id = ?", rowMapper, id)
            .firstOrNull()

    // Recent jobs, newest first, for the reports-ui jobs view (RFC-0002 D5).
    // The caller clamps limit to a bounded range (the controller), so this is
    // never an unbounded scan; created_at DESC is index-backed
    // (report_jobs_created_at_idx).
    fun list(limit: Int): List<ReportJob> =
        jdbc.query(
            "SELECT * FROM report_jobs ORDER BY created_at DESC LIMIT ?",
            rowMapper,
            limit,
        )

    private val rowMapper =
        RowMapper { rs: ResultSet, _: Int ->
            ReportJob(
                id = rs.getString("id"),
                type = ReportType.fromWire(rs.getString("type")) ?: error("unknown report type in row"),
                format = ReportFormat.fromWire(rs.getString("format")) ?: error("unknown format in row"),
                status = JobStatus.valueOf(rs.getString("status")),
                params = parseParams(rs.getString("params")),
                createdAt = rs.getTimestamp("created_at").toInstant(),
                startedAt = rs.getTimestamp("started_at")?.toInstant(),
                finishedAt = rs.getTimestamp("finished_at")?.toInstant(),
                error = rs.getString("error"),
                artifactPath = rs.getString("artifact_path"),
                artifactBytes = rs.getObject("artifact_bytes")?.let { (it as Number).toLong() },
            )
        }

    private fun parseParams(json: String?): Map<String, Any?> {
        if (json.isNullOrBlank()) return emptyMap()
        @Suppress("UNCHECKED_CAST")
        return mapper.readValue(json, Map::class.java) as Map<String, Any?>
    }
}
