package com.bench.ktor

import com.bench.ktor.routes.dbRoutes
import com.bench.ktor.routes.paramsRoutes
import com.bench.ktor.routes.rootRoutes
import com.bench.ktor.routes.webRoutes
import com.bench.shared.Consts
import com.bench.shared.Env
import com.bench.shared.db.Repositories
import freemarker.cache.ClassTemplateLoader
import io.ktor.serialization.kotlinx.json.json
import io.ktor.server.application.Application
import io.ktor.server.application.ApplicationStopped
import io.ktor.server.application.install
import io.ktor.server.freemarker.FreeMarker
import io.ktor.server.plugins.bodylimit.RequestBodyLimit
import io.ktor.server.plugins.calllogging.CallLogging
import io.ktor.server.plugins.contentnegotiation.ContentNegotiation
import io.ktor.server.plugins.statuspages.StatusPages
import io.ktor.server.routing.routing
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json

/**
 * Request/response Json shared by every route:
 *  - `ignoreUnknownKeys = true` — a wrongly-cased PATCH key is ignored (no-op update),
 *    while a missing required create field still fails (MissingFieldException → 400).
 *  - `explicitNulls = false` — a null `favoriteNumber` is dropped on encode, so the
 *    contract's absent-vs-0 distinction holds.
 * Duplicate keys resolve last-wins by default (verified in JsonSemanticsTest).
 */
val appJson: Json =
    Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

/**
 * The Ktor application module — referenced from `application.yaml` and started by
 * `EngineMain`, which reads the port from config and installs the graceful
 * SIGINT/SIGTERM shutdown hook automatically (kotlin.md item 22). DB pools/sessions
 * are opened once here and closed on `ApplicationStopped` (after the server drains).
 */
fun Application.module() {
    val env = Env.load()
    val repositories = runBlocking { Repositories.connect(env) }
    monitor.subscribe(ApplicationStopped) { repositories.disconnect() }

    install(ContentNegotiation) { json(appJson) }
    install(FreeMarker) {
        templateLoader = ClassTemplateLoader(Application::class.java.classLoader, "templates")
    }
    // Global 10 MiB request-body cap (matches the other servers); the /params/file
    // route enforces its own tighter 1 MiB per-file cap in the handler (413).
    install(RequestBodyLimit) { bodyLimit { Consts.MAX_REQUEST_BYTES } }
    install(StatusPages) { apiStatusPages() }
    // Logger off in prod (repo mandate): per-request logging only in dev.
    if (!env.isProd) {
        install(CallLogging)
    }

    routing {
        rootRoutes()
        paramsRoutes()
        dbRoutes(repositories)
        webRoutes(env.jwtSecret)
    }
}
