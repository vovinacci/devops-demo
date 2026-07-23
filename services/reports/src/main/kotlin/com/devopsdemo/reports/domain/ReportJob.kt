package com.devopsdemo.reports.domain

import java.time.Instant

// A report job row (report_jobs). Immutable snapshot read from the store;
// state transitions are separate persisted UPDATEs in ReportJobRepository, not
// mutations of this object.
data class ReportJob(
    val id: String,
    val type: ReportType,
    val format: ReportFormat,
    val status: JobStatus,
    val params: Map<String, Any?>,
    val createdAt: Instant,
    val startedAt: Instant?,
    val finishedAt: Instant?,
    val error: String?,
    val artifactPath: String?,
    val artifactBytes: Long?,
)
