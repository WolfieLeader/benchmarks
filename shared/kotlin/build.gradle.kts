// :shared — framework-independent infrastructure for the Kotlin benchmark servers
// (kt-ktor now, kt-spring-boot later): DB repositories, the user model + validation
// rules, the env contract, canonical constants, and the web-suite services (JWT,
// compute, validation schema). Routing/handlers/app wiring stay per-framework
// (PLAN §3: "sharing stops where idiom starts").
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ktlint)
    alias(libs.plugins.detekt)
}

// Compile on JDK 21 (LTS, reproducibility-safe pin) regardless of the Gradle
// launcher JVM (kotlin.md item 41: launcher-newer-than-toolchain is supported).
kotlin { jvmToolchain(21) }

repositories { mavenCentral() }

dependencies {
    // Coroutines + kotlinx.serialization leak through the repository API (suspend
    // methods, @Serializable model), so expose them transitively with `api`.
    api(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.serialization.json)

    // DB drivers + web-suite libs are internal to the repositories/services.
    implementation(libs.hikari)
    implementation(libs.postgresql)
    implementation(libs.mongodb.driver.kotlin.coroutine)
    implementation(libs.lettuce.core)
    implementation(libs.cassandra.java.driver.core)
    implementation(libs.java.uuid.generator)
    implementation(libs.java.jwt)

    testImplementation(kotlin("test"))
}

tasks.withType<Test>().configureEach { useJUnitPlatform() }

detekt {
    buildUponDefaultConfig = true
    // One config per language at the language root (PLAN §0.1 lint/format row).
    config.setFrom(rootProject.file("config/detekt/detekt.yml"))
}
