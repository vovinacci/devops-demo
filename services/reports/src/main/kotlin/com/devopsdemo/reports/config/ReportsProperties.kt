package com.devopsdemo.reports.config

import org.springframework.boot.context.properties.ConfigurationProperties
import java.time.Duration

// Report engine configuration bound from the `reports.*` block in
// application.yml (RFC-0001 D2/D10). backendUrl is the source of truth reports
// pulls items from; analyticsUrl is best-effort enrichment that degrades
// rather than fails a job (Hard rule 9). artifactDir is the named volume where
// generated files land -- never the DB (RFC-0001 D2).
@ConfigurationProperties(prefix = "reports")
data class ReportsProperties(
    val artifactDir: String,
    val backendUrl: String,
    val analyticsUrl: String,
    val httpTimeout: Duration,
    // Fixed-size dispatcher width: how many report jobs render concurrently.
    // Bounded so bursty POI/PDF allocation is a controlled GC exhibit, not an
    // OOM (RFC-0001 D2).
    val jobConcurrency: Int,
)
