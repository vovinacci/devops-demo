package com.devopsdemo.reports

import com.devopsdemo.reports.config.ReportsProperties
import org.springframework.boot.autoconfigure.SpringBootApplication
import org.springframework.boot.context.properties.EnableConfigurationProperties
import org.springframework.boot.runApplication

@SpringBootApplication
@EnableConfigurationProperties(ReportsProperties::class)
class ReportsApplication

fun main(args: Array<String>) {
    runApplication<ReportsApplication>(*args)
}
