package com.devopsdemo.reports.render

import com.devopsdemo.reports.data.ItemsSummaryModel
import com.devopsdemo.reports.domain.ReportFormat
import org.springframework.stereotype.Component

// One renderer per output format, all consuming the same ItemsSummaryModel so
// the three artifacts carry identical logical content (RFC-0001 D2).
interface ReportRenderer {
    val format: ReportFormat

    fun render(model: ItemsSummaryModel): ByteArray
}

// Resolves the renderer for a requested format. Fails loudly if a format has
// no renderer -- a wiring bug, not a client error (POST validation already
// rejected unknown formats before a job is ever created).
@Component
class ReportRenderers(
    renderers: List<ReportRenderer>,
) {
    private val byFormat: Map<ReportFormat, ReportRenderer> = renderers.associateBy { it.format }

    fun forFormat(format: ReportFormat): ReportRenderer = byFormat[format] ?: error("no renderer registered for format $format")
}
