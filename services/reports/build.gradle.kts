plugins {
    alias(libs.plugins.spring.boot)
    alias(libs.plugins.dependency.management)
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.spring)
    alias(libs.plugins.ktlint)
}

group = "com.devopsdemo"
version = "0.1.0"

kotlin {
    jvmToolchain(21)
    compilerOptions {
        freeCompilerArgs.add("-Xjsr305=strict")
    }
}

ktlint {
    version.set(libs.versions.ktlint)
}

repositories {
    mavenCentral()
}

// Dependency locking (RFC-0001 D6 audit): pins every resolved dependency,
// transitive included, into gradle.lockfile -- a reproducible-resolution
// guarantee, and the artifact the reports.yml Trivy audit enumerates for
// CVEs (Trivy's fs jar analyzer does not descend a Spring Boot executable
// jar; the lockfile is the reliable source-level dependency inventory).
// Regenerate after any dependency change with: ./gradlew dependencies --write-locks
dependencyLocking {
    lockAllConfigurations()
}

dependencies {
    implementation("org.springframework.boot:spring-boot-starter-web")
    implementation("org.springframework.boot:spring-boot-starter-actuator")
    implementation("org.springframework.boot:spring-boot-starter-jdbc")
    // Flyway schema migrations for report_jobs (PR-2, RFC-0001 D2: "the
    // heavyweight framework is the lesson"). flyway-database-postgresql is a
    // required companion since Flyway 10 -- the Postgres dialect moved out of
    // flyway-core. Both versions are Boot-BOM-managed.
    implementation("org.flywaydb:flyway-core")
    implementation("org.flywaydb:flyway-database-postgresql")
    // Prometheus exposition at /metrics (D6) and W3C trace context in logs
    // (D11): Micrometer meter registry + the OpenTelemetry tracing bridge.
    // No OTLP exporter is on the classpath, so spans are created and dropped
    // -- trace_id/span_id reach Loki via the JSON logs, Tempo is deferred.
    implementation("io.micrometer:micrometer-registry-prometheus")
    implementation("io.micrometer:micrometer-tracing-bridge-otel")
    implementation("com.fasterxml.jackson.module:jackson-module-kotlin")
    implementation("org.jetbrains.kotlin:kotlin-reflect")
    // Structured concurrency for the async job runner (RFC-0001 D2
    // coroutines): the POST returns immediately, the job runs off-thread on a
    // bounded dispatcher. Version is Boot-BOM-managed (kotlinx-coroutines-bom).
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core")
    // Coroutine handler + Reactor context bridge (RFC-0001 D2 coroutines).
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-reactor")
    // Report generators (PR-2): XLSX via Apache POI -- also the deliberate
    // bursty-allocation workload behind the GC-sawtooth exhibit -- and PDF via
    // OpenPDF. CSV needs no library. Both pinned in the version catalog.
    implementation(libs.poi.ooxml)
    implementation(libs.openpdf)
    implementation(libs.logstash.logback.encoder)
    // Pinned via the version catalog to override the Boot BOM (42.7.11 ->
    // 42.7.12, CVE-2026-54291): an explicit dependency version wins over the
    // BOM-managed one.
    runtimeOnly(libs.postgresql)

    testImplementation("org.springframework.boot:spring-boot-starter-test")
    testImplementation("org.springframework.boot:spring-boot-testcontainers")
    testImplementation("org.testcontainers:junit-jupiter")
    testImplementation("org.testcontainers:postgresql")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

tasks.withType<Test> {
    useJUnitPlatform()
}
