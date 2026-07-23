package com.devopsdemo.reports

import org.assertj.core.api.Assertions.assertThat
import org.junit.jupiter.api.MethodOrderer
import org.junit.jupiter.api.Order
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestMethodOrder
import org.springframework.beans.factory.annotation.Autowired
import org.springframework.boot.test.context.SpringBootTest
import org.springframework.boot.test.web.client.TestRestTemplate
import org.springframework.boot.testcontainers.service.connection.ServiceConnection
import org.springframework.http.HttpStatus
import org.testcontainers.containers.PostgreSQLContainer
import org.testcontainers.junit.jupiter.Container
import org.testcontainers.junit.jupiter.Testcontainers
import org.testcontainers.utility.DockerImageName

// Integration test against a real Postgres in a container -- the repo's
// testing philosophy (engineering-principles.md Section 7) is integration
// tests against real dependencies, not mocks; analytics tests do the same.
// @ServiceConnection wires spring.datasource at the container's URL, so the
// same startup path the compose `reports` profile takes is exercised here.
//
// Ordered so the disruptive DB-down test runs last: it stops the shared
// container, which nothing after it could reuse.
@Testcontainers
@TestMethodOrder(MethodOrderer.OrderAnnotation::class)
@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ReportsApplicationTests {
    companion object {
        // Digest-pinned to the same Postgres 16 image as postgres-reports in
        // compose. Referenced by digest without a tag: @ServiceConnection's
        // name deduction rejects the tag+digest form, and the digest is the
        // reproducible pin the repo standardizes on anyway.
        private val POSTGRES_IMAGE =
            DockerImageName.parse(
                "postgres@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777",
            )

        @Container
        @ServiceConnection
        @JvmStatic
        val postgres = PostgreSQLContainer(POSTGRES_IMAGE)
    }

    @Autowired
    lateinit var rest: TestRestTemplate

    @Test
    @Order(1)
    fun contextLoads() {
    }

    @Test
    @Order(2)
    fun healthzReturnsOk() {
        val response = rest.getForEntity("/healthz", String::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
    }

    @Test
    @Order(3)
    fun readyzReturnsOkWhenDatabaseUp() {
        val response = rest.getForEntity("/readyz", String::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
    }

    @Test
    @Order(4)
    fun metricsAreInPrometheusFormat() {
        val response = rest.getForEntity("/metrics", String::class.java)
        assertThat(response.statusCode).isEqualTo(HttpStatus.OK)
        assertThat(response.body).contains("jvm_memory_used_bytes")
    }

    // Proves the `db` datasource indicator is actually wired into the
    // readiness group (readinessState alone would keep /readyz at 200 with
    // the DB down, so an up-only assertion cannot prove the wiring). Stop
    // Postgres, then readiness must flip to 503 while liveness stays 200 --
    // liveness never depends on the database (RFC-0001 D6/D10).
    @Test
    @Order(5)
    fun readyzReflectsDatabaseDownWhileHealthzStaysUp() {
        postgres.stop()

        var readyStatus = HttpStatus.OK
        val deadline = System.currentTimeMillis() + 15_000
        while (System.currentTimeMillis() < deadline) {
            readyStatus = rest.getForEntity("/readyz", String::class.java).statusCode as HttpStatus
            if (readyStatus == HttpStatus.SERVICE_UNAVAILABLE) {
                break
            }
            Thread.sleep(500)
        }

        assertThat(readyStatus).isEqualTo(HttpStatus.SERVICE_UNAVAILABLE)
        assertThat(rest.getForEntity("/healthz", String::class.java).statusCode)
            .isEqualTo(HttpStatus.OK)
    }
}
