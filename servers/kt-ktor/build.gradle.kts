// kt-ktor — the Ktor (Netty) implementation of the 16-route API + the web suite.
// Consumes :shared for all infrastructure; only routing/handlers/app wiring live
// here (PLAN §3). The `application` plugin produces the runnable distribution the
// Dockerfile copies (`./gradlew :kt-ktor:installDist`).
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ktlint)
    alias(libs.plugins.detekt)
    application
}

kotlin { jvmToolchain(21) }

repositories { mavenCentral() }

dependencies {
    implementation(project(":shared"))

    implementation(platform(libs.ktor.bom))
    implementation(libs.ktor.server.core)
    implementation(libs.ktor.server.netty)
    implementation(libs.ktor.server.content.negotiation)
    implementation(libs.ktor.serialization.kotlinx.json)
    implementation(libs.ktor.server.status.pages)
    implementation(libs.ktor.server.call.logging)
    implementation(libs.ktor.server.freemarker)
    implementation(libs.ktor.server.config.yaml)
    implementation(libs.ktor.server.body.limit)
    runtimeOnly(libs.logback.classic)

    // In-process route tests (testApplication) for the behaviors the contract
    // does not pin case-by-case: the 413 mapping and the file-sniff branches.
    testImplementation(kotlin("test"))
    testImplementation(libs.ktor.server.test.host)
}

tasks.withType<Test>().configureEach { useJUnitPlatform() }

application {
    // Ktor's EngineMain reads application.yaml (module + deployment.port) and
    // installs the graceful SIGINT/SIGTERM shutdown hook automatically (kotlin.md
    // item 22).
    mainClass.set("io.ktor.server.netty.EngineMain")
}

detekt {
    buildUponDefaultConfig = true
    config.setFrom(rootProject.file("config/detekt/detekt.yml"))
}
