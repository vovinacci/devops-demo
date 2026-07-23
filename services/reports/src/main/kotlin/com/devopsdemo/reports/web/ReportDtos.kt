package com.devopsdemo.reports.web

import com.devopsdemo.reports.domain.ReportJob
import java.time.Instant

// POST /reports request. type + format are required and validated (400 on bad
// input); params is a free-form map passed to the report type.
data class CreateReportRequest(
    val type: String?,
    val format: String?,
    val params: Map<String, Any?> = emptyMap(),
)

// 202 Accepted body. The Location header points at the status resource; this
// mirrors it in the body for clients that do not read headers (e.g. the k6
// scenario in PR-3).
data class JobAcceptedResponse(
    val id: String,
    val status: String,
    val location: String,
)

// GET /reports/{id} body: the job's current state. download is populated only
// once the job SUCCEEDED and an artifact exists.
data class JobStatusResponse(
    val id: String,
    val type: String,
    val format: String,
    val status: String,
    val createdAt: Instant,
    val startedAt: Instant?,
    val finishedAt: Instant?,
    val error: String?,
    val artifactBytes: Long?,
    val download: String?,
) {
    companion object {
        fun from(job: ReportJob): JobStatusResponse =
            JobStatusResponse(
                id = job.id,
                type = job.type.wire,
                format = job.format.wire,
                status = job.status.name,
                createdAt = job.createdAt,
                startedAt = job.startedAt,
                finishedAt = job.finishedAt,
                error = job.error,
                artifactBytes = job.artifactBytes,
                download = if (job.artifactPath != null) "/reports/${job.id}/download" else null,
            )
    }
}
