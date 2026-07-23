package com.devopsdemo.reports.jobs

import com.devopsdemo.reports.config.ReportsProperties
import com.devopsdemo.reports.domain.ReportFormat
import org.slf4j.LoggerFactory
import org.springframework.stereotype.Component
import java.nio.file.Files
import java.nio.file.Path

// Persists generated artifacts to the named artifact volume (RFC-0001 D2:
// files on the volume, never BLOBs in the DB). The artifact_path stored in
// report_jobs points back here; the download endpoint reads it back out.
@Component
class ArtifactStore(
    props: ReportsProperties,
) {
    private val log = LoggerFactory.getLogger(javaClass)
    private val root: Path = Path.of(props.artifactDir).toAbsolutePath().normalize()

    init {
        Files.createDirectories(root)
        log.info("artifact directory: {}", root)
    }

    // Writes the artifact and returns its absolute path. The job id names the
    // file, so at most one artifact exists per job.
    fun write(
        jobId: String,
        format: ReportFormat,
        bytes: ByteArray,
    ): Path {
        val target = root.resolve("$jobId.${format.extension}")
        Files.write(target, bytes)
        return target
    }

    // Resolves a stored artifact path for download, guarding against a path
    // that escapes the artifact root (defence in depth -- the path always
    // comes from a row this service wrote, never from the client).
    fun resolveExisting(artifactPath: String): Path? {
        val path = Path.of(artifactPath).toAbsolutePath().normalize()
        if (!path.startsWith(root)) {
            log.warn("refusing artifact path outside root: {}", artifactPath)
            return null
        }
        return if (Files.isReadable(path)) path else null
    }
}
