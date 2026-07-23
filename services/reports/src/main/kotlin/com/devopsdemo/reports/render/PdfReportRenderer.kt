package com.devopsdemo.reports.render

import com.devopsdemo.reports.data.ItemsSummaryModel
import com.devopsdemo.reports.domain.ReportFormat
import com.lowagie.text.Document
import com.lowagie.text.Element
import com.lowagie.text.Paragraph
import com.lowagie.text.pdf.PdfPCell
import com.lowagie.text.pdf.PdfPTable
import com.lowagie.text.pdf.PdfWriter
import org.springframework.stereotype.Component
import java.io.ByteArrayOutputStream
import java.time.format.DateTimeFormatter

// PDF rendering via OpenPDF (RFC-0001 D2). Same logical content as the
// XLSX/CSV outputs: a summary block, an items table, and the analytics section
// (numbers when available, a note when degraded, RFC-0001 D10).
@Component
class PdfReportRenderer : ReportRenderer {
    override val format = ReportFormat.PDF

    override fun render(model: ItemsSummaryModel): ByteArray {
        val out = ByteArrayOutputStream()
        val document = Document()
        try {
            PdfWriter.getInstance(document, out)
            document.open()

            document.add(Paragraph("Items Summary Report"))
            document.add(Paragraph("Generated at: ${DateTimeFormatter.ISO_INSTANT.format(model.generatedAt)}"))
            document.add(Paragraph("Total items: ${model.totalItems}"))
            document.add(Paragraph("Synthetic items: ${model.syntheticItems}"))
            document.add(Paragraph("Real items: ${model.realItems}"))
            document.add(Paragraph(" "))

            document.add(Paragraph("Items"))
            val itemsTable = PdfPTable(3)
            headerCells(itemsTable, "id", "name", "synthetic")
            for (item in model.items) {
                itemsTable.addCell(item.id.toString())
                itemsTable.addCell(item.name)
                itemsTable.addCell(item.synthetic.toString())
            }
            document.add(itemsTable)
            document.add(Paragraph(" "))

            document.add(Paragraph("Analytics (last 24h events by type)"))
            if (model.analytics.available) {
                val analyticsTable = PdfPTable(2)
                headerCells(analyticsTable, "event_type", "count")
                for (stat in model.analytics.stats) {
                    analyticsTable.addCell(stat.eventType)
                    analyticsTable.addCell(stat.count.toString())
                }
                document.add(analyticsTable)
            } else {
                document.add(Paragraph(model.analytics.note ?: "Analytics unavailable."))
            }
        } finally {
            // Release the writer/document even if a mid-render add throws, so a
            // failed render does not leak native/stream resources.
            if (document.isOpen) document.close()
        }
        return out.toByteArray()
    }

    private fun headerCells(
        table: PdfPTable,
        vararg names: String,
    ) {
        for (name in names) {
            val cell = PdfPCell(Paragraph(name))
            cell.horizontalAlignment = Element.ALIGN_CENTER
            table.addCell(cell)
        }
    }
}
