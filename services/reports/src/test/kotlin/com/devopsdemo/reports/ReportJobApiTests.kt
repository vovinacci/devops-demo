package com.devopsdemo.reports

import com.devopsdemo.reports.domain.JobStatus
import com.devopsdemo.reports.domain.ReportFormat
import com.devopsdemo.reports.domain.ReportType
import com.devopsdemo.reports.jobs.ReportJobRepository
import com.devopsdemo.reports.jobs.ReportService
import com.lowagie.text.pdf.PdfReader
import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import org.apache.poi.xssf.usermodel.XSSFWorkbook
import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.AfterAll
import org.junit.jupiter.api.BeforeAll
import org.junit.jupiter.api.MethodOrderer
import org.junit.jupiter.api.Order
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestMethodOrder
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.boot.test.web.client.TestRestTemplate
import org.springframework.boot.testcontainers.service.connection.ServiceConnection
import org.springframework.http.HttpStatus
import org.springframework.test.context.DynamicPropertyRegistry
import org.springframework.test.context.DynamicPropertySource
import org.testcontainers.containers.PostgreSQLContainer
import org.testcontainers.junit.jupiter.Container
import org.testcontainers.junit.jupiter.Testcontainers
import org.testcontainers.utility.DockerImageName
import java.io.ByteArrayInputStream
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import java.nio.file.Files
import java.time.Instant
import java.util.UUID

// End-to-end tests for the report job engine against real dependencies (repo
// philosophy, engineering-principles.md Section 7): Postgres via Testcontainers
// (Flyway migrates it at startup), and the backend/analytics upstreams via a
// lightweight JDK HttpServer stub so no network or WireMock dependency is
// pulled in. Each format is POSTed, polled to SUCCEEDED, downloaded, and
// parse-validated; the D10 analytics-absent and analytics-malformed paths, a
// job-failure path, CSV formula neutralization, and startup reconciliation are
// exercised too.
//
// The stub response bodies are mutable so a single test can inject a malformed
// or formula-bearing payload and restore it. Tests that stop a stub server
// (analytics-down, backend-down) run last -- nothing after them relies on that
// server being up.
@Testcontainers
@TestMethodOrder(MethodOrderer.OrderAnnotation::class)
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ReportJobApiTests {
    companion object {
        private val POSTGRES_IMAGE =
            DockerImageName.parse(
                "postgres@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777",
            )

        @Container
        @ServiceConnection
        @JvmStatic
        val postgres = PostgreSQLContainer(POSTGRES_IMAGE)

        // Default backend body: one real item plus two synthetic (loadgen-,
        // canary- prefixes, RFC-0001 D9) so synthetic tagging is asserted.
        private const val DEFAULT_BACKEND_ITEMS =
            """[{"id":1,"name":"widget"},{"id":2,"name":"loadgen-abc"},{"id":3,"name":"canary-xyz"}]"""
        private const val DEFAULT_ANALYTICS_STATS =
            """[{"event_type":"created","count":5},{"event_type":"deleted","count":2}]"""

        // Mutable so a single test can inject a malformed/formula payload and
        // restore the default in a finally.
        @Volatile private var backendBody = DEFAULT_BACKEND_ITEMS

        @Volatile private var analyticsBody = DEFAULT_ANALYTICS_STATS

        private lateinit var backend: HttpServer
        private lateinit var analytics: HttpServer

        private fun respond(
            exchange: HttpExchange,
            payload: String,
        ) {
            val body = payload.toByteArray(StandardCharsets.UTF_8)
            exchange.responseHeaders.add("Content-Type", "application/json")
            exchange.sendResponseHeaders(200, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }

        @BeforeAll
        @JvmStatic
        fun startStubs() {
            backend = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
            backend.createContext("/items") { exchange -> respond(exchange, backendBody) }
            backend.start()

            analytics = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
            analytics.createContext("/api/v1/stats") { exchange -> respond(exchange, analyticsBody) }
            analytics.start()
        }

        @AfterAll
        @JvmStatic
        fun stopStubs() {
            backend.stop(0)
            analytics.stop(0)
        }

        @DynamicPropertySource
        @JvmStatic
        fun properties(registry: DynamicPropertyRegistry) {
            registry.add("reports.backend-url") { "http://127.0.0.1:${backend.address.port}" }
            registry.add("reports.analytics-url") { "http://127.0.0.1:${analytics.address.port}" }
            registry.add("reports.http-timeout") { "2s" }
            registry.add("reports.artifact-dir") { Files.createTempDirectory("reports-test").toString() }
        }
    }

    @Autowired
    lateinit var rest: TestRestTemplate

    @Autowired
    lateinit var reportService: ReportService

    @Autowired
    lateinit var repository: ReportJobRepository

    private fun submit(format: String): String {
        val response =
            rest.postForEntity(
                "/reports",
                mapOf("type" to "items-summary", "format" to format),
                Map::class.java,
            )
        assertThat(response.statusCode).isEqualTo(HttpStatus.ACCEPTED)
        assertThat(response.headers.location).isNotNull()
        return response.body!!["id"] as String
    }

    private fun awaitStatus(
        id: String,
        target: String,
    ): Map<*, *> {
        val deadline = System.currentTimeMillis() + 20_000
        while (System.currentTimeMillis() < deadline) {
            val body = rest.getForEntity("/reports/$id", Map::class.java).body!!
            val status = body["status"] as String
            if (status == target) return body
            if (status == "FAILED" && target != "FAILED") {
                error("job $id failed unexpectedly: ${body["error"]}")
            }
            Thread.sleep(200)
        }
        error("job $id did not reach $target in time")
    }

    private fun downloadText(id: String): String = String(rest.getForEntity("/reports/$id/download", ByteArray::class.java).body!!, StandardCharsets.UTF_8)

    @Test
    @Order(1)
    fun csvJobSucceedsAndDownloads() {
        val id = submit("csv")
        val status = awaitStatus(id, "SUCCEEDED")
        assertThat(status["download"]).isEqualTo("/reports/$id/download")

        val response = rest.getForEntity("/reports/$id/download", ByteArray::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
        assertThat(response.headers.contentType.toString()).contains("text/csv")
        assertThat(response.headers.getFirst("Content-Disposition")).contains("$id.csv")
        val text = String(response.body!!, StandardCharsets.UTF_8)
        assertThat(text).contains("id,name,synthetic")
        assertThat(text).contains("widget")
        assertThat(text).contains("total_items")
        // Analytics section present with real numbers (analytics up).
        assertThat(text).contains("event_type,count")
        assertThat(text).contains("created")
    }

    @Test
    @Order(2)
    fun xlsxJobSucceedsAndParses() {
        val id = submit("xlsx")
        awaitStatus(id, "SUCCEEDED")

        val response = rest.getForEntity("/reports/$id/download", ByteArray::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
        XSSFWorkbook(ByteArrayInputStream(response.body!!)).use { workbook ->
            assertThat(workbook.getSheet("Summary")).isNotNull()
            assertThat(workbook.getSheet("Items")).isNotNull()
            assertThat(workbook.getSheet("Analytics")).isNotNull()
            val items = workbook.getSheet("Items")
            // header + 3 item rows
            assertThat(items.lastRowNum).isEqualTo(3)
            assertThat(items.getRow(1).getCell(1).stringCellValue).isEqualTo("widget")
        }
    }

    @Test
    @Order(3)
    fun pdfJobSucceedsAndParses() {
        val id = submit("pdf")
        awaitStatus(id, "SUCCEEDED")

        val response = rest.getForEntity("/reports/$id/download", ByteArray::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
        assertThat(response.headers.contentType.toString()).contains("application/pdf")
        val bytes = response.body!!
        assertThat(String(bytes.copyOfRange(0, 5), StandardCharsets.US_ASCII)).isEqualTo("%PDF-")
        val reader = PdfReader(bytes)
        assertThat(reader.numberOfPages).isGreaterThanOrEqualTo(1)
        reader.close()
    }

    @Test
    @Order(4)
    fun csvNeutralizesFormulaInjection() {
        // An item name that would be a spreadsheet formula must be neutralized
        // in the CSV (prefixed so the cell is inert). XLSX/PDF are not formula-
        // evaluated, so this guard is CSV-only.
        backendBody = """[{"id":1,"name":"=1+2"},{"id":2,"name":"@SUM(A1:A2)"}]"""
        try {
            val id = submit("csv")
            awaitStatus(id, "SUCCEEDED")
            val text = downloadText(id)
            // The dangerous leading char is prefixed with an apostrophe; the raw
            // formula never appears as a line-leading cell.
            assertThat(text).contains("'=1+2")
            assertThat(text).contains("'@SUM(A1:A2)")
            assertThat(text).doesNotContain("\n=1+2")
        } finally {
            backendBody = DEFAULT_BACKEND_ITEMS
        }
    }

    @Test
    @Order(5)
    fun unknownFormatReturns400() {
        val response =
            rest.postForEntity(
                "/reports",
                mapOf("type" to "items-summary", "format" to "docx"),
                Map::class.java,
            )
        assertThat(response.statusCode).isEqualTo(HttpStatus.BAD_REQUEST)
    }

    @Test
    @Order(6)
    fun unknownTypeReturns400() {
        val response =
            rest.postForEntity(
                "/reports",
                mapOf("type" to "nope", "format" to "csv"),
                Map::class.java,
            )
        assertThat(response.statusCode).isEqualTo(HttpStatus.BAD_REQUEST)
    }

    @Test
    @Order(7)
    fun unknownJobReturns404() {
        val response = rest.getForEntity("/reports/does-not-exist", Map::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.NOT_FOUND)
    }

    @Test
    @Order(8)
    fun malformedAnalyticsDegradesButStillSucceeds() {
        // D10 graceful degradation (Hard rule 9): analytics returns HTTP 200 but
        // a body that is not the expected array. The job must still SUCCEED on
        // backend data with the unavailable marker, not fail on a parse error.
        analyticsBody = """{"unexpected":"shape"}"""
        try {
            val id = submit("csv")
            awaitStatus(id, "SUCCEEDED")
            val text = downloadText(id)
            assertThat(text).contains("widget")
            assertThat(text).contains("analytics_status")
            assertThat(text).contains("unavailable")
        } finally {
            analyticsBody = DEFAULT_ANALYTICS_STATS
        }
    }

    @Test
    @Order(9)
    fun analyticsDownStillSucceedsWithMarker() {
        // D10: stop analytics entirely (connection refused), the job must still
        // SUCCEED on backend data with a distinct unavailable marker.
        analytics.stop(0)

        val id = submit("csv")
        awaitStatus(id, "SUCCEEDED")
        val text = downloadText(id)
        assertThat(text).contains("widget")
        assertThat(text).contains("analytics_status")
        assertThat(text).contains("unavailable")
    }

    @Test
    @Order(10)
    fun backendDownFailsJobAndDownloadConflicts() {
        // Backend is the source of truth: with it down the job FAILS, surfaces
        // the error, and download is 409 (no artifact to serve).
        backend.stop(0)

        val id = submit("csv")
        val status = awaitStatus(id, "FAILED")
        assertThat(status["error"]).isNotNull()
        assertThat(status["download"]).isNull()

        val download = rest.getForEntity("/reports/$id/download", String::class.java)
        assertThat(download.statusCode).isEqualTo(HttpStatus.CONFLICT)
    }

    @Test
    @Order(11)
    fun startupReconciliationFailsOrphanedRunningJob() {
        // A job left RUNNING by a crash/interrupted shutdown must be reconciled
        // to FAILED on startup. Simulate the orphan (insert + markRunning) and
        // invoke the same sweep the ApplicationReadyEvent listener runs.
        val id = UUID.randomUUID().toString()
        repository.insertPending(id, ReportType.ITEMS_SUMMARY, ReportFormat.CSV, emptyMap())
        repository.markRunning(id, Instant.now())
        assertThat(repository.find(id)!!.status).isEqualTo(JobStatus.RUNNING)

        reportService.reconcileOrphanedJobs()

        val reconciled = repository.find(id)!!
        assertThat(reconciled.status).isEqualTo(JobStatus.FAILED)
        assertThat(reconciled.error).contains("reconciled on startup")
    }
}
