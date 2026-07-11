// Root Gradle settings — THE Kotlin-lane root-contention file (PLAN §11.1). The
// Kotlin servers do NOT live under one directory (flat `servers/` layout, §2.1),
// so each subproject maps its projectDir explicitly. A second Kotlin consumer
// (kt-spring-boot) joins by appending its own include(...) + projectDir line and
// a subproject build.gradle.kts — no restructuring of what is here.
rootProject.name = "benchmarks"

pluginManagement {
    repositories {
        gradlePluginPortal()
        mavenCentral()
    }
}

dependencyResolutionManagement {
    repositories {
        mavenCentral()
    }
}

include(":shared")
project(":shared").projectDir = file("shared/kotlin")

include(":kt-ktor")
project(":kt-ktor").projectDir = file("servers/kt-ktor")
