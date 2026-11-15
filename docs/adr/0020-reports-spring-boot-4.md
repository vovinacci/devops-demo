# ADR-0020: reports moves to Spring Boot 4

- Status: Accepted
- Date: 2026-10-12
- Supersedes: the "Spring Boot 3" version pin in RFC-0001 D2 (Section 5.1
  service table, D2 stack list, repository layout comment). The rest of D2
  -- Kotlin, Spring MVC over WebFlux, JDK 21 virtual threads, coroutines for
  the job runner, the heavyweight-framework rationale -- stands unchanged.

## Context

RFC-0001 D2 chose Spring Boot 3 for the reports service, the JVM exhibit of
the platform. In October 2026 two CRITICAL CVEs landed in Spring Framework
6.2.19 (`spring-webmvc`): CVE-2026-47884 (remote code execution through
`XsltView`) and CVE-2026-47890 (SSE stream corruption with view rendering).
6.2.19 is the last open-source 6.2.x release; the only fix is Spring
Framework 7.0.9, which ships with Spring Boot 4.

The Trivy gate (ADR-0011) fails on fixable CRITICAL findings in images we
build. #253 suppressed both CVEs until 2026-11-12, on the grounds that
neither code path is used, to give this migration its own change. That
suppression is a loan, not a decision: staying on Boot 3 means every future
Spring CVE arrives with no open-source fix, and the gate either goes red or
fills with suppressions.

## Decision

The reports service moves to Spring Boot 4.1 (Spring Framework 7, Jakarta
EE 11, Tomcat 11, Jackson 3), with Kotlin on the version Boot 4.1 manages.

The shape of the service does not change: Spring MVC on virtual threads,
JDBC + Flyway against its own Postgres, coroutines for the job runner,
Micrometer/Prometheus at `/metrics`, the Actuator `/healthz` and `/readyz`
split, JSON logs with `trace_id`/`span_id`. The uniform service contract
(RFC-0001 D6, Hard rule 4) is preserved, not amended.

What Boot 4 changes, and how this repo takes it:

- **Modular auto-configuration.** Boot 4 splits auto-configuration into
  per-technology modules. Flyway needs `spring-boot-starter-flyway` --
  `flyway-core` alone no longer runs migrations at startup, silently.
  Tracing uses `spring-boot-micrometer-tracing-opentelemetry` with the
  Micrometer OTel bridge, not the OpenTelemetry starter: the starter adds
  OTLP exporters, and spans in this repo stay unexported until Tempo lands.
- **Jackson 3.** `com.fasterxml.jackson` becomes `tools.jackson`; the
  Logstash encoder moves to its Jackson 3 line (9.x).
- **Test support.** `TestRestTemplate` moves to `spring-boot-resttestclient`
  and needs `@AutoConfigureTestRestTemplate`; Testcontainers 2 renames its
  modules (`testcontainers-postgresql`, `testcontainers-junit-jupiter`).

## Alternatives

- **Stay on Boot 3.5 with suppressions:** rejected. The CVEs are not
  reachable today, but the 6.2 line gets no further open-source fixes, so
  the gate would accumulate suppressions for a framework nobody patches.
  That teaches the opposite of ADR-0011.
- **Commercial Spring support for 6.2.x:** rejected -- a paid dependency
  for a teaching repo, and students could not reproduce the build.
- **Swap to a lighter framework (Ktor):** rejected for the reason D2 chose
  Spring in the first place -- the framework's weight is the lesson.

## Consequences

- Easier: the Spring line is back on supported open-source releases; the
  two suppressions are gone and `.trivyignore.yaml` is empty.
- Easier: Boot 4's explicit modules make the dependency list say what the
  service uses.
- Harder: Boot 4's module split fails quietly when a starter is missing --
  a missing Flyway starter still compiles and boots, it just skips the
  migrations. The Testcontainers suite catches this (it exercises the
  migrated schema); keep it.
- Harder: Kotlin, ktlint and the Logstash encoder now move with Boot's
  major line rather than independently.
- Unchanged: Tomcat stays overridden through the BOM's `tomcat.version`
  property when Boot lags a Tomcat security release (now on the 11.0.x
  line).
