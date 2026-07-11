package com.bench.spring.config

import com.bench.shared.Env
import com.bench.shared.db.Repositories
import com.bench.spring.appJson
import kotlinx.coroutines.runBlocking
import org.springframework.context.annotation.Bean
import org.springframework.context.annotation.Configuration
import org.springframework.http.codec.ServerCodecConfigurer
import org.springframework.http.codec.json.KotlinSerializationJsonDecoder
import org.springframework.http.codec.json.KotlinSerializationJsonEncoder
import org.springframework.web.reactive.config.WebFluxConfigurer

/** Application beans: the env contract + the four connected DB repositories. */
@Configuration
class AppConfig {
    @Bean
    fun env(): Env = Env.load()

    /**
     * Connect the four DB repositories once at startup — `runBlocking` is correct here
     * (bootstrap, not a request path; item 13's ban is on coroutine/handler threads).
     * `disconnect` is the destroy method: Spring runs it on context close, AFTER the
     * graceful web-server drain (earliest SmartLifecycle phase), so the pools outlive
     * in-flight requests.
     */
    @Bean(destroyMethod = "disconnect")
    fun repositories(env: Env): Repositories = runBlocking { Repositories.connect(env) }
}

/**
 * Register the app-wide kotlinx.serialization Json (ignoreUnknownKeys +
 * explicitNulls=false) as THE JSON codec, replacing WebFlux's auto-configured default
 * so `favoriteNumber: null` is dropped on encode (absent-vs-0 contract rule).
 */
@Configuration
class WebConfig : WebFluxConfigurer {
    override fun configureHttpMessageCodecs(configurer: ServerCodecConfigurer) {
        val defaults = configurer.defaultCodecs()
        defaults.kotlinSerializationJsonDecoder(KotlinSerializationJsonDecoder(appJson))
        defaults.kotlinSerializationJsonEncoder(KotlinSerializationJsonEncoder(appJson))
    }
}
