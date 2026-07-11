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

// Each Kotlin server ships a Docker image built from a context that copies ONLY
// shared/kotlin + that one server's dir (per-server Dockerfiles, PLAN §2.4). Gradle
// validates every configured projectDir at settings time, so guard each server include
// on its dir being present: on the full checkout both resolve, while inside a single
// server's image only that server's include activates (the other's dir is absent).
if (file("servers/kt-ktor").isDirectory) {
    include(":kt-ktor")
    project(":kt-ktor").projectDir = file("servers/kt-ktor")
}

if (file("servers/kt-spring-boot").isDirectory) {
    include(":kt-spring-boot")
    project(":kt-spring-boot").projectDir = file("servers/kt-spring-boot")
}
