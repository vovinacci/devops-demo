package com.devopsdemo.reports.render

import com.devopsdemo.reports.data.ItemsSummaryModel
import com.devopsdemo.reports.domain.ReportFormat
import org.springframework.stereotype.Component
import java.time.format.DateTimeFormatter

// CSV rendering: the items table (id,name,synthetic) as the primary block,
// then a blank line and the analytics block -- both RFC 4180 quoted. Flat by
// nature, but carries the same logical content as the XLSX/PDF outputs.
@Component
class CsvReportRenderer : ReportRenderer {
    override val format = ReportFormat.CSV

    override fun render(model: ItemsSummaryModel): ByteArray {
        val sb = StringBuilder()

        // Summary preamble as key,value rows.
        sb.appendCsv("section", "value")
        sb.appendCsv("generated_at", DateTimeFormatter.ISO_INSTANT.format(model.generatedAt))
        sb.appendCsv("total_items", model.totalItems.toString())
        sb.appendCsv("synthetic_items", model.syntheticItems.toString())
        sb.appendCsv("real_items", model.realItems.toString())
        sb.append("\r\n")

        // Items table.
        sb.appendCsv("id", "name", "synthetic")
        for (item in model.items) {
            sb.appendCsv(item.id.toString(), item.name, item.synthetic.toString())
        }
        sb.append("\r\n")

        // Analytics block -- present as data when available, as a single note
        // row when degraded (RFC-0001 D10).
        if (model.analytics.available) {
            sb.appendCsv("event_type", "count")
            for (stat in model.analytics.stats) {
                sb.appendCsv(stat.eventType, stat.count.toString())
            }
        } else {
            sb.appendCsv("analytics_status", "note")
            sb.appendCsv("unavailable", model.analytics.note ?: "analytics unavailable")
        }

        return sb.toString().toByteArray(Charsets.UTF_8)
    }

    private fun StringBuilder.appendCsv(vararg fields: String) {
        append(fields.joinToString(",") { escape(it) })
        append("\r\n")
    }

    // Item names come from backend data (attacker-influenceable). A field whose
    // first character triggers spreadsheet formula evaluation (= + - @, or a
    // leading tab/CR that some parsers strip to expose the next char) is
    // neutralized with a leading apostrophe so Excel/LibreOffice treat the cell
    // as inert text -- then the usual RFC 4180 quoting is applied. XLSX (POI
    // setCellValue(String)) and PDF are not formula-evaluated, so this guard is
    // CSV-only.
    private fun escape(field: String): String {
        val neutralized = if (field.isNotEmpty() && field.first() in FORMULA_TRIGGERS) "'$field" else field
        return if (neutralized.any { it == ',' || it == '"' || it == '\n' || it == '\r' }) {
            "\"" + neutralized.replace("\"", "\"\"") + "\""
        } else {
            neutralized
        }
    }

    private companion object {
        val FORMULA_TRIGGERS = setOf('=', '+', '-', '@', '\t', '\r')
    }
}
