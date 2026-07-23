package com.devopsdemo.reports.render

import com.devopsdemo.reports.data.ItemsSummaryModel
import com.devopsdemo.reports.domain.ReportFormat
import org.apache.poi.ss.usermodel.Row
import org.apache.poi.ss.usermodel.Sheet
import org.apache.poi.ss.usermodel.Workbook
import org.apache.poi.xssf.usermodel.XSSFWorkbook
import org.springframework.stereotype.Component
import java.io.ByteArrayOutputStream
import java.time.format.DateTimeFormatter

// XLSX rendering via Apache POI (RFC-0001 D2). POI's in-memory workbook
// building is the deliberate bursty-allocation workload that makes the GC
// sawtooth visible on the Reports JVM dashboard -- the reason this service
// exists. Three sheets (Summary, Items, Analytics) mirror the CSV/PDF blocks.
@Component
class XlsxReportRenderer : ReportRenderer {
    override val format = ReportFormat.XLSX

    override fun render(model: ItemsSummaryModel): ByteArray =
        XSSFWorkbook().use { workbook ->
            writeSummary(workbook, model)
            writeItems(workbook, model)
            writeAnalytics(workbook, model)
            ByteArrayOutputStream().use { out ->
                workbook.write(out)
                out.toByteArray()
            }
        }

    private fun writeSummary(
        workbook: Workbook,
        model: ItemsSummaryModel,
    ) {
        val sheet = workbook.createSheet("Summary")
        header(sheet.createRow(0), "field", "value")
        keyValue(sheet, 1, "generated_at", DateTimeFormatter.ISO_INSTANT.format(model.generatedAt))
        keyValue(sheet, 2, "total_items", model.totalItems.toString())
        keyValue(sheet, 3, "synthetic_items", model.syntheticItems.toString())
        keyValue(sheet, 4, "real_items", model.realItems.toString())
    }

    private fun writeItems(
        workbook: Workbook,
        model: ItemsSummaryModel,
    ) {
        val sheet = workbook.createSheet("Items")
        header(sheet.createRow(0), "id", "name", "synthetic")
        model.items.forEachIndexed { index, item ->
            val row = sheet.createRow(index + 1)
            row.createCell(0).setCellValue(item.id.toDouble())
            row.createCell(1).setCellValue(item.name)
            row.createCell(2).setCellValue(item.synthetic)
        }
    }

    private fun writeAnalytics(
        workbook: Workbook,
        model: ItemsSummaryModel,
    ) {
        val sheet = workbook.createSheet("Analytics")
        if (model.analytics.available) {
            header(sheet.createRow(0), "event_type", "count")
            model.analytics.stats.forEachIndexed { index, stat ->
                val row = sheet.createRow(index + 1)
                row.createCell(0).setCellValue(stat.eventType)
                row.createCell(1).setCellValue(stat.count.toDouble())
            }
        } else {
            // D10 degraded section: a single note instead of numbers.
            header(sheet.createRow(0), "analytics_status", "note")
            keyValue(sheet, 1, "unavailable", model.analytics.note ?: "analytics unavailable")
        }
    }

    private fun header(
        row: Row,
        vararg names: String,
    ) {
        names.forEachIndexed { index, name -> row.createCell(index).setCellValue(name) }
    }

    private fun keyValue(
        sheet: Sheet,
        rowIndex: Int,
        key: String,
        value: String,
    ) {
        val row = sheet.createRow(rowIndex)
        row.createCell(0).setCellValue(key)
        row.createCell(1).setCellValue(value)
    }
}
