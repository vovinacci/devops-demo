package com.devopsdemo.reports.data

import java.time.Instant

// One logical report, format-agnostic: the XLSX, PDF, and CSV renderers all
// consume this same model so the three outputs carry identical content
// (RFC-0001 D2). Built by ReportModelBuilder from backend items (source of
// truth) enriched with analytics aggregates (best-effort, D10).

data class ItemRow(
    val id: Long,
    val name: String,
    // Synthetic traffic (canary journeys, loadgen CRUD) is tagged by name
    // prefix so it can be labeled distinctly instead of silently inflating the
    // business figures (RFC-0001 D9). Flagged here, surfaced in every format.
    val synthetic: Boolean,
)

data class AnalyticsStat(
    val eventType: String,
    val count: Long,
)

// The analytics enrichment section. available=false is the D10 degraded path:
// analytics was absent or unreachable, so the report still succeeded on
// backend data alone and this section carries a note instead of numbers --
// never a job failure (Hard rule 9).
data class AnalyticsSection(
    val available: Boolean,
    val stats: List<AnalyticsStat>,
    val note: String?,
)

data class ItemsSummaryModel(
    val generatedAt: Instant,
    val items: List<ItemRow>,
    val analytics: AnalyticsSection,
) {
    val totalItems: Int get() = items.size
    val syntheticItems: Int get() = items.count { it.synthetic }
    val realItems: Int get() = totalItems - syntheticItems
}
