package com.devopsdemo.reports.data

import com.devopsdemo.reports.config.ReportsProperties
import com.fasterxml.jackson.databind.JsonNode
import com.fasterxml.jackson.databind.ObjectMapper
import org.springframework.stereotype.Component
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse

// Synthetic-traffic tagging (RFC-0001 D9): canary journeys and loadgen CRUD
// create items under these name prefixes so they can be flowed through the
// real pipeline yet labeled apart from business data in reports/dashboards.
object SyntheticTraffic {
    private val prefixes = listOf("canary-", "loadgen-")

    fun isSynthetic(name: String): Boolean = prefixes.any { name.startsWith(it) }
}

// JsonNode.get(field) returns a NullNode (not Kotlin null) for an explicit
// {"field": null}, and Kotlin null only for a missing field -- so a plain
// `?: throw` catches the missing case but silently lets an explicit null
// through to asLong()/asText() (which coerce it to 0/""). A required field is
// absent in either case, so both upstream parsers guard with this.
private fun JsonNode?.isAbsentOrNull(): Boolean = this == null || this.isNull || this.isMissingNode

// Thrown when the backend (the source of truth) cannot be read. Unlike an
// analytics failure this is fatal to the job: without items there is no report
// (RFC-0001 D2, contrast Hard rule 9 which covers only the optional analytics
// upstream).
class BackendUnavailableException(
    message: String,
    cause: Throwable? = null,
) : RuntimeException(message, cause)

// Backend REST client: pulls the item list that the report is built from.
@Component
class BackendClient(
    private val http: HttpClient,
    private val mapper: ObjectMapper,
    private val props: ReportsProperties,
) {
    fun listItems(): List<ItemRow> {
        val request =
            HttpRequest
                .newBuilder(URI.create("${props.backendUrl.trimEnd('/')}/items"))
                .timeout(props.httpTimeout)
                .GET()
                .build()

        val response =
            try {
                http.send(request, HttpResponse.BodyHandlers.ofString())
            } catch (e: Exception) {
                throw BackendUnavailableException("backend request to ${props.backendUrl} failed", e)
            }

        if (response.statusCode() !in 200..299) {
            throw BackendUnavailableException("backend returned HTTP ${response.statusCode()}")
        }

        // The backend is the source of truth: a malformed body or a missing
        // required field is a hard failure (fail the job with a clear error),
        // never a silently-empty "0 items" report -- contrast the analytics
        // client, which degrades.
        val root =
            try {
                mapper.readTree(response.body())
            } catch (e: Exception) {
                throw BackendUnavailableException("backend /items returned a non-JSON body", e)
            }
        if (!root.isArray) {
            throw BackendUnavailableException("backend /items did not return a JSON array")
        }
        return root.map { node ->
            val idNode = node.get("id")
            if (idNode.isAbsentOrNull()) throw BackendUnavailableException("backend item missing 'id'")
            val nameNode = node.get("name")
            if (nameNode.isAbsentOrNull()) throw BackendUnavailableException("backend item missing 'name'")
            val name = nameNode!!.asText()
            ItemRow(
                id = idNode!!.asLong(),
                name = name,
                synthetic = SyntheticTraffic.isSynthetic(name),
            )
        }
    }
}

// Thrown when analytics cannot be read. Caught by ReportModelBuilder, which
// then produces the D10 degraded analytics section rather than failing the job.
class AnalyticsUnavailableException(
    message: String,
    cause: Throwable? = null,
) : RuntimeException(message, cause)

// Analytics REST client: fetches the last-24h event-type aggregates that
// enrich the report. Optional upstream -- callers treat any failure as
// "analytics unavailable", not a job failure (Hard rule 9).
@Component
class AnalyticsClient(
    private val http: HttpClient,
    private val mapper: ObjectMapper,
    private val props: ReportsProperties,
) {
    fun stats(): List<AnalyticsStat> {
        val request =
            HttpRequest
                .newBuilder(URI.create("${props.analyticsUrl.trimEnd('/')}/api/v1/stats"))
                .timeout(props.httpTimeout)
                .GET()
                .build()

        val response =
            try {
                http.send(request, HttpResponse.BodyHandlers.ofString())
            } catch (e: Exception) {
                throw AnalyticsUnavailableException("analytics request to ${props.analyticsUrl} failed", e)
            }

        if (response.statusCode() !in 200..299) {
            throw AnalyticsUnavailableException("analytics returned HTTP ${response.statusCode()}")
        }

        // Analytics is best-effort (D10, Hard rule 9): a 200 with a malformed or
        // unexpectedly-shaped body must still degrade the report, not fail the
        // job -- so the parse runs inside the same AnalyticsUnavailableException
        // contract as a transport failure, never leaking a raw parse exception
        // past ReportModelBuilder.fetchAnalytics.
        return try {
            val root = mapper.readTree(response.body())
            require(root.isArray) { "analytics /api/v1/stats did not return a JSON array" }
            root.map { node ->
                val typeNode = node.get("event_type")
                require(!typeNode.isAbsentOrNull()) { "analytics stat missing 'event_type'" }
                val countNode = node.get("count")
                require(!countNode.isAbsentOrNull()) { "analytics stat missing 'count'" }
                AnalyticsStat(eventType = typeNode!!.asText(), count = countNode!!.asLong())
            }
        } catch (e: Exception) {
            throw AnalyticsUnavailableException("analytics response was not the expected shape", e)
        }
    }
}
