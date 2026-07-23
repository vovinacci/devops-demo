package com.devopsdemo.reports.jobs

import com.devopsdemo.reports.config.ReportsProperties
import com.devopsdemo.reports.data.ReportModelBuilder
import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportJob
import com.devopsdemo.reports.domain.ReportType
import com.devopsdemo.reports.render.ReportRenderers
import kotlinx.coroutines.CoroutineName
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.plus
import kotlinx.coroutines.withContext
import org.slf4j.LoggerFactory
import org.springframework.beans.factory.DisposableBean
import org.springframework.boot.context.event.ApplicationReadyEvent
import org.springframework.context.event.EventListener
import org.springframework.stereotype.Service
import java.time.Duration
import java.time.Instant
import java.util.UUID
import java.util.concurrent.CancellationException
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

// Async report job runner (RFC-0001 D2 coroutines). POST /reports calls
// submit(), which persists a PENDING job and returns immediately; the render
// runs off-thread on a bounded dispatcher via structured concurrency. Every
// state transition is persisted (ReportJobRepository) so a job is traceable
// end to end (D11) and never lost on crash/shutdown.
@Service
class ReportService(
    private val repository: ReportJobRepository,
    private val modelBuilder: ReportModelBuilder,
    private val renderers: ReportRenderers,
    private val artifactStore: ArtifactStore,
    private val metrics: ReportMetrics,
    props: ReportsProperties,
) : DisposableBean {
    private val log = LoggerFactory.getLogger(javaClass)

    // Fixed-size platform-thread pool: report jobs (bursty POI/PDF allocation +
    // blocking upstream HTTP + JDBC) run here, bounded by reports.job-concurrency
    // so allocation is a controlled GC exhibit, not an OOM (RFC-0001 D2).
    private val executor: ExecutorService =
        Executors.newFixedThreadPool(props.jobConcurrency) { runnable ->
            Thread(runnable, "report-job").apply { isDaemon = true }
        }
    private val scope =
        CoroutineScope(SupervisorJob() + executor.asCoroutineDispatcher() + CoroutineName("report-jobs"))

    fun submit(
        type: ReportType,
        format: ReportFormat,
        params: Map<String, Any?>,
    ): String {
        val id = UUID.randomUUID().toString()
        repository.insertPending(id, type, format, params)
        metrics.jobSubmitted()
        log.info("report job accepted id={} type={} format={}", id, type.wire, format.wire)
        scope.launch { runJob(id, type, format) }
        return id
    }

    fun find(id: String): ReportJob? = repository.find(id)

    private suspend fun runJob(
        id: String,
        type: ReportType,
        format: ReportFormat,
    ) {
        // Increment inside runJob so it is paired with the finally decrement in
        // the same lifecycle: if the launch is ever rejected, runJob never runs
        // and the gauge is untouched (the row stays PENDING, reconciled on the
        // next startup) -- no permanent over-count.
        metrics.inflightIncrement()
        val startNanos = System.nanoTime()
        var status = JobStatus.FAILED
        try {
            repository.markRunning(id, Instant.now())
            log.info("report job started id={}", id)

            val model = modelBuilder.build(type)
            val bytes = renderers.forFormat(format).render(model)
            val path = artifactStore.write(id, format, bytes)

            repository.markSucceeded(id, path.toString(), bytes.size.toLong(), Instant.now())
            metrics.artifactWritten(bytes.size.toLong())
            status = JobStatus.SUCCEEDED
            log.info("report job succeeded id={} bytes={} path={}", id, bytes.size, path)
        } catch (e: CancellationException) {
            // Best-effort only: this is reached solely if cancellation is
            // observed at a suspension point, and the blocking POI/PDF render
            // has none. It is NOT the guarantee -- reconcileOrphanedJobs() on the
            // next startup is what guarantees no job stays non-terminal.
            withContext(NonCancellable) {
                repository.markFailed(id, "interrupted by shutdown", Instant.now())
            }
            log.warn("report job interrupted by shutdown id={}", id)
            throw e
        } catch (e: Exception) {
            val message = e.message ?: e.javaClass.simpleName
            repository.markFailed(id, message, Instant.now())
            log.error("report job failed id={}: {}", id, message, e)
        } finally {
            metrics.inflightDecrement()
            metrics.jobCompleted(status, type, format, Duration.ofNanos(System.nanoTime() - startNanos))
        }
    }

    // Graceful shutdown (RFC-0001 D2): stop accepting new jobs and let in-flight
    // ones drain for up to DRAIN_SECONDS. Jobs that finish within the grace
    // complete normally. A CPU-bound POI/PDF render still running past the grace
    // is interrupted, but it may ignore the interrupt (and the worker threads
    // are daemon), so a job still rendering at JVM exit is NOT marked FAILED
    // here -- it is reconciled to FAILED on the next startup
    // (reconcileOrphanedJobs). This method promises only the best-effort drain.
    override fun destroy() {
        log.info("shutting down report job scope, draining in-flight jobs")
        executor.shutdown()
        if (!executor.awaitTermination(DRAIN_SECONDS, TimeUnit.SECONDS)) {
            log.warn("in-flight report jobs did not drain in {}s, interrupting", DRAIN_SECONDS)
            scope.cancel()
            executor.shutdownNow()
        }
    }

    // On startup, reconcile any job left non-terminal (PENDING or RUNNING) by a
    // crash or an interrupted in-flight render. There is exactly one reports
    // instance (compose), so any such row at boot is by definition orphaned --
    // this is the durable-recovery guarantee the per-transition persistence
    // promises, and the real backstop for the shutdown edge case above.
    @EventListener(ApplicationReadyEvent::class)
    fun reconcileOrphanedJobs() {
        val reconciled =
            repository.failNonTerminal(
                "reconciled on startup: interrupted by shutdown or crash",
                Instant.now(),
            )
        if (reconciled > 0) {
            log.warn("reconciled {} orphaned report job(s) to FAILED on startup", reconciled)
        }
    }

    private companion object {
        const val DRAIN_SECONDS = 10L
    }
}
