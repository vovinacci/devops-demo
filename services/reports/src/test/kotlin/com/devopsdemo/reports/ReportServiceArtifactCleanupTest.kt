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
import com.devopsdemo.reports.render.CsvReportRenderer
import com.devopsdemo.reports.render.ReportRenderers
import com.fasterxml.jackson.databind.ObjectMapper
import io.micrometer.core.instrument.simple.SimpleMeterRegistry
import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.Test
import org.springframework.jdbc.core.JdbcTemplate
import java.net.http.HttpClient
import java.nio.file.Files
import java.nio.file.Path
import java.time.Duration
import java.time.Instant

// Finding 3 (PR #178): when the artifact write succeeds but the subsequent
// markSucceeded UPDATE fails, the job ends FAILED but must NOT leave an
// orphaned artifact on the volume (the download endpoint never serves a FAILED
// job, so the file is pure waste that leaks storage on repeated transient DB
// failures). This is a plain unit test -- no Spring context, no Postgres -- so
// it can inject a repository whose markSucceeded throws deterministically.
class ReportServiceArtifactCleanupTest {
    // In-memory repository. markSucceeded always throws (the transient DB
    // failure under test); every other transition is recorded so find() can be
    // polled for the terminal state. Subclassable because the kotlin-spring
    // allopen plugin opens @Repository classes and their members.
    private class ThrowingRepository : ReportJobRepository(JdbcTemplate(), ObjectMapper()) {
        private val statuses = HashMap<String, JobStatus>()
        private val errors = HashMap<String, String>()

        @Volatile var markSucceededCalled = false

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
            markSucceededCalled = true
            throw RuntimeException("simulated transient DB failure on markSucceeded")
        }

        override fun markFailed(
            id: String,
            error: String,
            finishedAt: Instant,
        ) {
            synchronized(this) {
                statuses[id] = JobStatus.FAILED
                errors[id] = error
            }
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
                    error = errors[id],
                    artifactPath = null,
                    artifactBytes = null,
                )
            }
    }

    @Test
    fun failedMarkSucceededDeletesOrphanedArtifactAndFailsJob() {
        val tmp = Files.createTempDirectory("reports-cleanup-test")
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
        // build() is overridden so the upstream clients are never called.
        val modelBuilder =
            object : ReportModelBuilder(BackendClient(http, mapper, props), AnalyticsClient(http, mapper, props)) {
                override fun build(type: ReportType): ItemsSummaryModel =
                    ItemsSummaryModel(
                        generatedAt = Instant.now(),
                        items = listOf(ItemRow(id = 1, name = "widget", synthetic = false)),
                        analytics = AnalyticsSection(available = false, stats = emptyList(), note = "n/a"),
                    )
            }
        val renderers = ReportRenderers(listOf(CsvReportRenderer()))
        val artifactStore = ArtifactStore(props)
        val metrics = ReportMetrics(SimpleMeterRegistry())
        val repository = ThrowingRepository()
        val service = ReportService(repository, modelBuilder, renderers, artifactStore, metrics, props)

        try {
            val id = service.submit(ReportType.ITEMS_SUMMARY, ReportFormat.CSV, emptyMap())
            awaitFailed(repository, id)

            assertThat(repository.markSucceededCalled).isTrue()
            assertThat(repository.find(id)!!.status).isEqualTo(JobStatus.FAILED)

            val artifact: Path = tmp.resolve("$id.csv")
            assertThat(Files.exists(artifact))
                .`as`("orphaned artifact must be deleted when the job fails after write")
                .isFalse()
        } finally {
            service.destroy()
        }
    }

    private fun awaitFailed(
        repository: ReportJobRepository,
        id: String,
    ) {
        val deadline = System.currentTimeMillis() + 5_000
        while (System.currentTimeMillis() < deadline) {
            if (repository.find(id)?.status == JobStatus.FAILED) return
            Thread.sleep(25)
        }
        error("job $id did not reach FAILED in time")
    }
}
