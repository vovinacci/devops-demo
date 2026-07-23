package com.devopsdemo.reports.jobs

import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportType
import io.micrometer.core.instrument.DistributionSummary
import io.micrometer.core.instrument.MeterRegistry
import io.micrometer.core.instrument.Timer
import org.springframework.stereotype.Component
import java.time.Duration
import java.util.concurrent.atomic.AtomicInteger

// Custom report-job meters that feed the Reports JVM dashboard (RFC-0001 D2,
// Section 7), alongside the JVM/GC meters Micrometer exposes for the sawtooth
// exhibit. Prometheus names: reports_jobs_submitted_total,
// reports_jobs_completed_total{status,type,format}, reports_job_duration_seconds
// (histogram), reports_jobs_inflight, reports_artifact_bytes.
@Component
class ReportMetrics(
    private val registry: MeterRegistry,
) {
    private val inflight = AtomicInteger(0)

    private val submitted = registry.counter("reports.jobs.submitted")

    // No baseUnit set deliberately: the Prometheus registry appends baseUnit to
    // the meter name, which would make this reports_artifact_bytes_bytes_sum.
    // The name already carries the unit, so the series stays reports_artifact_bytes_sum.
    private val artifactBytes: DistributionSummary =
        DistributionSummary
            .builder("reports.artifact.bytes")
            .description("Size of generated report artifacts in bytes")
            .register(registry)

    init {
        registry.gauge("reports.jobs.inflight", inflight) { it.get().toDouble() }
    }

    fun jobSubmitted() {
        submitted.increment()
    }

    fun inflightIncrement() {
        inflight.incrementAndGet()
    }

    fun inflightDecrement() {
        inflight.decrementAndGet()
    }

    fun jobCompleted(
        status: JobStatus,
        type: ReportType,
        format: ReportFormat,
        duration: Duration,
    ) {
        registry
            .counter(
                "reports.jobs.completed",
                "status",
                status.name.lowercase(),
                "type",
                type.wire,
                "format",
                format.wire,
            ).increment()
        Timer
            .builder("reports.job.duration")
            .tag("status", status.name.lowercase())
            .tag("type", type.wire)
            .tag("format", format.wire)
            .register(registry)
            .record(duration)
    }

    fun artifactWritten(bytes: Long) {
        artifactBytes.record(bytes.toDouble())
    }
}
