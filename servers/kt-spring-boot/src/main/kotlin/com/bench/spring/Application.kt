package com.bench.spring

import kotlinx.serialization.json.Json
import org.springframework.boot.SpringApplication
import org.springframework.boot.autoconfigure.SpringBootApplication

/**
 * Request/response Json shared by every route and the WebFlux codec (registered in
 * [com.bench.spring.config.WebConfig]):
 *  - `ignoreUnknownKeys = true` — a wrongly-cased PATCH key is ignored (no-op update),
 *    while a missing required create field still fails (MissingFieldException → 400).
 *  - `explicitNulls = false` — a null `favoriteNumber` is dropped on encode, so the
 *    contract's absent-vs-0 distinction holds.
 * Duplicate keys resolve last-wins by default (kotlinx.serialization behaviour proven
 * in :shared's JsonSemanticsTest). Matches kt-ktor's `appJson` exactly.
 */
val appJson: Json =
    Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

@SpringBootApplication
class Application

/**
 * Boot entry point. `ENV=prod` (the env-var contract) activates the `prod` Spring
 * profile so `application.yaml` drops logging to WARN — Spring profiles are not wired
 * to arbitrary env vars, so map it explicitly here rather than depending on
 * SPRING_PROFILES_ACTIVE. Graceful SIGINT/SIGTERM shutdown is handled by Spring Boot's
 * registered shutdown hook (`server.shutdown: graceful`).
 *
 * `@Suppress("SpreadOperator")`: `SpringApplication.run(vararg)` is the framework entry
 * point — spreading argv is the only way to forward it, unavoidable and one-shot.
 */
@Suppress("SpreadOperator")
fun main(args: Array<String>) {
    val app = SpringApplication(Application::class.java)
    if (System.getenv("ENV") == "prod") app.setAdditionalProfiles("prod")
    app.run(*args)
}
