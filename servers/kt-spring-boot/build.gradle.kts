// kt-spring-boot — the Spring Boot 4.1 (WebFlux + Kotlin coroutines) implementation
// of the 16-route API + the web suite. Consumes :shared for all infrastructure;
// only routing/handlers/app wiring live here (PLAN §3).
//
// WebFlux (not MVC): :shared's repositories are coroutine-native (`suspend fun` across
// all four backends), so suspend controllers consume them directly with zero bridging
// — the idiomatic match. kotlin.md item 20's "MVC + virtual threads" guidance was
// premised on a blocking JDBC layer, which the shipped :shared module is not.
//
// kotlinx.serialization (not Jackson): the shared model is @Serializable, and Spring 7
// gates its Kotlin-serialization codec on @Serializable types (KotlinDetector), so it
// reproduces the exact JSON semantics the contract requires (duplicate-key last-wins,
// explicitNulls=false for absent-vs-0, required-field-missing → 400, fractional→Int
// rejected). spring-boot-starter-json (Jackson) is therefore excluded.
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.kotlin.spring)
    alias(libs.plugins.spring.boot)
    alias(libs.plugins.ktlint)
    alias(libs.plugins.detekt)
    application
}

kotlin { jvmToolchain(21) }

repositories { mavenCentral() }

dependencies {
    implementation(project(":shared"))

    // Spring Boot BOM as a Gradle platform (mirrors the Ktor BOM usage) — manages
    // every spring-boot-* + transitive version, so the starters below carry none.
    implementation(platform(libs.spring.boot.dependencies))

    implementation(libs.spring.boot.starter.webflux) {
        // Drop Jackson — kotlinx.serialization is the JSON stack (see file header).
        exclude(group = "org.springframework.boot", module = "spring-boot-starter-json")
    }
    implementation(libs.spring.boot.starter.thymeleaf)

    // Coroutine support for WebFlux (suspend controllers) needs the Reactor bridge on
    // the classpath. Both coroutine artifacts are pinned to the same catalog version
    // so -core and -reactor can never skew.
    implementation(libs.kotlinx.coroutines.core)
    implementation(libs.kotlinx.coroutines.reactor)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(kotlin("test"))
    testImplementation(libs.spring.boot.starter.test)
}

tasks.withType<Test>().configureEach { useJUnitPlatform() }

application {
    // The `application` plugin supplies the `run` task the roster's `just dev` command
    // invokes (`:kt-spring-boot:run`); the Docker image ships the bootJar instead.
    mainClass.set("com.bench.spring.ApplicationKt")
}

// A fixed jar name so the Dockerfile copies one deterministic artifact.
tasks.named<org.springframework.boot.gradle.tasks.bundling.BootJar>("bootJar") {
    archiveFileName.set("app.jar")
}

detekt {
    buildUponDefaultConfig = true
    // One config per language at the language root (PLAN §0.1 lint/format row).
    config.setFrom(rootProject.file("config/detekt/detekt.yml"))
}
