package com.devopsdemo.reports.web

import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportType
import com.devopsdemo.reports.jobs.ArtifactStore
import com.devopsdemo.reports.jobs.ReportService
import com.devopsdemo.reports.jobs.ReportServiceUnavailableException
import org.springframework.core.io.FileSystemResource
import org.springframework.core.io.Resource
import org.springframework.http.HttpHeaders
import org.springframework.http.HttpStatus
import org.springframework.http.MediaType
import org.springframework.http.ResponseEntity
import org.springframework.web.bind.annotation.ExceptionHandler
import org.springframework.web.bind.annotation.GetMapping
import org.springframework.web.bind.annotation.PathVariable
import org.springframework.web.bind.annotation.PostMapping
import org.springframework.web.bind.annotation.RequestBody
import org.springframework.web.bind.annotation.RequestMapping
import org.springframework.web.bind.annotation.RequestParam
import org.springframework.web.bind.annotation.ResponseStatus
import org.springframework.web.bind.annotation.RestController
import org.springframework.web.server.ResponseStatusException
import java.net.URI

// Report job API (RFC-0001 D2). Async: POST returns 202 + a Location pointing
// at the status resource; the artifact is a separate download sub-resource so
// polling stays cheap and streaming the file is a distinct, correctly-typed
// response (the cleaner REST shape over content negotiation).
@RestController
@RequestMapping("/reports")
class ReportController(
    private val service: ReportService,
    private val artifactStore: ArtifactStore,
) {
    @PostMapping
    fun create(
        @RequestBody request: CreateReportRequest,
    ): ResponseEntity<JobAcceptedResponse> {
        val type =
            ReportType.fromWire(request.type.orEmpty())
                ?: throw ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "unknown report type '${request.type}', supported: ${ReportType.entries.joinToString { it.wire }}",
                )
        val format =
            ReportFormat.fromWire(request.format.orEmpty())
                ?: throw ResponseStatusException(
                    HttpStatus.BAD_REQUEST,
                    "unknown format '${request.format}', supported: ${ReportFormat.entries.joinToString { it.wire }}",
                )

        val id = service.submit(type, format, request.params)
        val location = "/reports/$id"
        return ResponseEntity
            .accepted()
            .location(URI.create(location))
            .body(JobAcceptedResponse(id = id, status = JobStatus.PENDING.name, location = location))
    }

    // List recent jobs, newest first, for the reports-ui jobs view (RFC-0002
    // D5). Metadata only, no artifacts. limit is clamped to [MIN_LIMIT,
    // MAX_LIMIT] so a client cannot request an unbounded scan; omitted -> the
    // default. Routes cleanly beside GET /{id}: the bare /reports path matches
    // here, /reports/{id} matches the status mapping below.
    @GetMapping
    fun list(
        @RequestParam(required = false) limit: Int?,
    ): List<JobSummaryResponse> {
        val clamped = (limit ?: DEFAULT_LIMIT).coerceIn(MIN_LIMIT, MAX_LIMIT)
        return service.list(clamped).map(JobSummaryResponse::from)
    }

    @GetMapping("/{id}")
    fun status(
        @PathVariable id: String,
    ): JobStatusResponse {
        val job = service.find(id) ?: throw notFound(id)
        return JobStatusResponse.from(job)
    }

    @GetMapping("/{id}/download")
    fun download(
        @PathVariable id: String,
    ): ResponseEntity<Resource> {
        val job = service.find(id) ?: throw notFound(id)
        if (job.status != JobStatus.SUCCEEDED || job.artifactPath == null) {
            // Not an error the client can retry away by re-requesting the same
            // URL immediately -- the artifact only exists once the job finished.
            throw ResponseStatusException(
                HttpStatus.CONFLICT,
                "report $id is ${job.status}, no artifact to download",
            )
        }
        val path =
            artifactStore.resolveExisting(job.artifactPath)
                ?: throw ResponseStatusException(HttpStatus.NOT_FOUND, "artifact for report $id is missing")

        val filename = "${job.id}.${job.format.extension}"
        return ResponseEntity
            .ok()
            .contentType(MediaType.parseMediaType(job.format.contentType))
            .header(HttpHeaders.CONTENT_DISPOSITION, "attachment; filename=\"$filename\"")
            .body(FileSystemResource(path))
    }

    private fun notFound(id: String) = ResponseStatusException(HttpStatus.NOT_FOUND, "report $id not found")

    // A submit rejected because the service is draining for shutdown: the job
    // was never persisted, so the client should retry later, not treat it as
    // accepted. 503 (with the retryable semantics that carries) is the right
    // signal, distinct from the 4xx client errors above.
    @ExceptionHandler(ReportServiceUnavailableException::class)
    @ResponseStatus(HttpStatus.SERVICE_UNAVAILABLE)
    fun handleShuttingDown(e: ReportServiceUnavailableException): Map<String, String> = mapOf("error" to (e.message ?: "service unavailable"))

    private companion object {
        // GET /reports?limit=N bounds: a sane default and a hard ceiling so a
        // client cannot request an unbounded number of rows (RFC-0002 D5).
        const val DEFAULT_LIMIT = 20
        const val MIN_LIMIT = 1
        const val MAX_LIMIT = 100
    }
}
