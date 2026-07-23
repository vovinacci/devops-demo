package com.devopsdemo.reports.data

import com.devopsdemo.reports.domain.ReportType
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component
import java.time.Instant

// Assembles the format-agnostic report model from the upstreams. Backend items
// are required (a failure propagates and fails the job); analytics enrichment
// is best-effort and degrades to an "unavailable" section on any error
// (RFC-0001 D10, Hard rule 9) -- the report still succeeds on backend data.
@Component
class ReportModelBuilder(
    private val backend: BackendClient,
    private val analytics: AnalyticsClient,
) {
    private val log = LoggerFactory.getLogger(javaClass)

    fun build(type: ReportType): ItemsSummaryModel =
        when (type) {
            ReportType.ITEMS_SUMMARY -> buildItemsSummary()
        }

    private fun buildItemsSummary(): ItemsSummaryModel {
        val items = backend.listItems()
        val analyticsSection = fetchAnalytics()
        return ItemsSummaryModel(
            generatedAt = Instant.now(),
            items = items,
            analytics = analyticsSection,
        )
    }

    private fun fetchAnalytics(): AnalyticsSection =
        try {
            AnalyticsSection(available = true, stats = analytics.stats(), note = null)
        } catch (e: AnalyticsUnavailableException) {
            // D10 graceful degradation: log and mark the section unavailable
            // rather than failing the job. The report ships with backend data
            // and a distinct note so the gap is visible, not hidden.
            log.warn("analytics enrichment unavailable, degrading report: {}", e.message)
            AnalyticsSection(
                available = false,
                stats = emptyList(),
                note = "Analytics unavailable at generation time -- report reflects backend data only (${e.message}).",
            )
        }
}
