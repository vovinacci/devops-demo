package com.devopsdemo.reports.config

import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import java.net.http.HttpClient

// Single shared JDK HttpClient for the backend/analytics REST calls. The JDK
// client keeps the outbound surface dependency-free (no WebClient/Reactor on
// the request path) and its blocking send() runs fine on the report jobs'
// bounded dispatcher. Per-request timeouts are set by the callers from
// reports.http-timeout; the connect timeout is set here to the same budget.
@Configuration
class HttpClientConfig {
    @Bean
    fun httpClient(props: ReportsProperties): HttpClient =
        HttpClient
            .newBuilder()
            .connectTimeout(props.httpTimeout)
            .build()
}
