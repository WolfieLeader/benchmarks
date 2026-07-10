// Root build — declares the shared plugin versions ONCE (via the version catalog)
// so every subproject applies them without repeating a version. Nothing is applied
// at the root itself; the actual config lives per subproject (kotlin.md item 40).
plugins {
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ktlint) apply false
    alias(libs.plugins.detekt) apply false
}
