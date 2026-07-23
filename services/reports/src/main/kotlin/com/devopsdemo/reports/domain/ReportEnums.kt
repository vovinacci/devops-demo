package com.devopsdemo.reports.domain

// Report output format. The wire value (lowercase) is what POST /reports
// accepts and what the report_jobs.format CHECK constraint stores; each format
// carries the MIME type and file extension used when streaming the artifact
// back on GET /reports/{id}/download.
enum class ReportFormat(
    val wire: String,
    val contentType: String,
    val extension: String,
) {
    XLSX("xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"),
    PDF("pdf", "application/pdf", "pdf"),
    CSV("csv", "text/csv", "csv"),
    ;

    companion object {
        fun fromWire(value: String): ReportFormat? = entries.firstOrNull { it.wire == value.lowercase() }
    }
}

// Report type selects what content is generated. ITEMS_SUMMARY is the one
// concrete report shipped in PR-2: backend items enriched with analytics
// aggregates (RFC-0001 D2). More types slot in here without touching the job
// machinery.
enum class ReportType(
    val wire: String,
) {
    ITEMS_SUMMARY("items-summary"),
    ;

    companion object {
        fun fromWire(value: String): ReportType? = entries.firstOrNull { it.wire == value.lowercase() }
    }
}

// Job state machine (persisted in report_jobs.status): PENDING (accepted) ->
// RUNNING (a worker picked it up) -> SUCCEEDED | FAILED. Terminal states carry
// either an artifact (SUCCEEDED) or an error (FAILED).
enum class JobStatus {
    PENDING,
    RUNNING,
    SUCCEEDED,
    FAILED,
    ;

    val terminal: Boolean
        get() = this == SUCCEEDED || this == FAILED
}
