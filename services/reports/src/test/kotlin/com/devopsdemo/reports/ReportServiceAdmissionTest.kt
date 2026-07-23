package com.devopsdemo.reports

import com.devopsdemo.reports.config.ReportsProperties
import com.devopsdemo.reports.data.AnalyticsClient
import com.devopsdemo.reports.data.AnalyticsSection
import com.devopsdemo.reports.data.BackendClient
import com.devopsdemo.reports.data.ItemRow
import com.devopsdemo.reports.data.ItemsSummaryModel
import com.devopsdemo.reports.data.ReportModelBuilder
import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportJob
import com.devopsdemo.reports.domain.ReportType
import com.devopsdemo.reports.jobs.ArtifactStore
import com.devopsdemo.reports.jobs.ReportJobRepository
import com.devopsdemo.reports.jobs.ReportMetrics
import com.devopsdemo.reports.jobs.ReportService
import com.devopsdemo.reports.jobs.ReportServiceUnavailableException
import com.devopsdemo.reports.render.CsvReportRenderer
import com.devopsdemo.reports.render.ReportRenderers
import com.fasterxml.jackson.databind.ObjectMapper
import io.micrometer.core.instrument.simple.SimpleMeterRegistry
import org.assertj.core.api.Assertions.assertThat
import org.assertj.core.api.Assertions.assertThatThrownBy
import org.junit.jupiter.api.Test
import org.springframework.jdbc.core.JdbcTemplate
import java.net.http.HttpClient
import java.nio.file.Files
import java.time.Duration
import java.time.Instant

// Finding (PR #178): job admission must be atomic with shutdown. Once destroy()
// has run, submit() must reject with ReportServiceUnavailableException (mapped
// to HTTP 503 by ReportController) rather than persist a PENDING row whose
// launch the drained executor would reject. A normal submit before shutdown
// must still be accepted. Plain unit test -- no Spring context, no Postgres.
class ReportServiceAdmissionTest {
    // In-memory repository recording every transition so a submitted job can be
    // polled to its terminal state. Subclassable because the kotlin-spring
    // allopen plugin opens @Repository classes and their members.
    private class RecordingRepository : ReportJobRepository(JdbcTemplate(), ObjectMapper()) {
        private val statuses = HashMap<String, JobStatus>()

        override fun insertPending(
            id: String,
            type: ReportType,
            format: ReportFormat,
            params: Map<String, Any?>,
        ) {
            synchronized(this) { statuses[id] = JobStatus.PENDING }
        }

        override fun markRunning(
            id: String,
            startedAt: Instant,
        ) {
            synchronized(this) { statuses[id] = JobStatus.RUNNING }
        }

        override fun markSucceeded(
            id: String,
            artifactPath: String,
            artifactBytes: Long,
            finishedAt: Instant,
        ) {
            synchronized(this) { statuses[id] = JobStatus.SUCCEEDED }
        }

        override fun markFailed(
            id: String,
            error: String,
            finishedAt: Instant,
        ) {
            synchronized(this) { statuses[id] = JobStatus.FAILED }
        }

        override fun find(id: String): ReportJob? =
            synchronized(this) {
                val status = statuses[id] ?: return null
                ReportJob(
                    id = id,
                    type = ReportType.ITEMS_SUMMARY,
                    format = ReportFormat.CSV,
                    status = status,
                    params = emptyMap(),
                    createdAt = Instant.now(),
                    startedAt = null,
                    finishedAt = null,
                    error = null,
                    artifactPath = null,
                    artifactBytes = null,
                )
            }
    }

    private fun newService(repository: ReportJobRepository): ReportService {
        val tmp = Files.createTempDirectory("reports-admission-test")
        val props =
            ReportsProperties(
                artifactDir = tmp.toString(),
                backendUrl = "http://unused",
                analyticsUrl = "http://unused",
                httpTimeout = Duration.ofSeconds(1),
                jobConcurrency = 1,
            )
        val http = HttpClient.newHttpClient()
        val mapper = ObjectMapper()
        val modelBuilder =
            object : ReportModelBuilder(BackendClient(http, mapper, props), AnalyticsClient(http, mapper, props)) {
                override fun build(type: ReportType): ItemsSummaryModel =
                    ItemsSummaryModel(
                        generatedAt = Instant.now(),
                        items = listOf(ItemRow(id = 1, name = "widget", synthetic = false)),
                        analytics = AnalyticsSection(available = false, stats = emptyList(), note = "n/a"),
                    )
            }
        return ReportService(
            repository,
            modelBuilder,
            ReportRenderers(listOf(CsvReportRenderer())),
            ArtifactStore(props),
            ReportMetrics(SimpleMeterRegistry()),
            props,
        )
    }

    @Test
    fun submitBeforeShutdownIsAcceptedAndRuns() {
        val repository = RecordingRepository()
        val service = newService(repository)
        try {
            val id = service.submit(ReportType.ITEMS_SUMMARY, ReportFormat.CSV, emptyMap())
            assertThat(id).isNotBlank()
            awaitStatus(repository, id, JobStatus.SUCCEEDED)
        } finally {
            service.destroy()
        }
    }

    @Test
    fun submitAfterShutdownThrowsUnavailable() {
        val repository = RecordingRepository()
        val service = newService(repository)
        service.destroy()

        assertThatThrownBy {
            service.submit(ReportType.ITEMS_SUMMARY, ReportFormat.CSV, emptyMap())
        }.isInstanceOf(ReportServiceUnavailableException::class.java)
    }

    private fun awaitStatus(
        repository: ReportJobRepository,
        id: String,
        target: JobStatus,
    ) {
        val deadline = System.currentTimeMillis() + 5_000
        while (System.currentTimeMillis() < deadline) {
            if (repository.find(id)?.status == target) return
            Thread.sleep(25)
        }
        error("job $id did not reach $target in time")
    }
}
