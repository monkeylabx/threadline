import org.gradle.api.artifacts.dsl.LockMode

plugins {
    base
    id("com.android.library") version "9.3.1" apply false
}

allprojects {
    dependencyLocking {
        lockAllConfigurations()
        lockMode.set(LockMode.STRICT)
    }
}

tasks.register("workspaceBuild") {
    group = "build"
    description = "Builds the Android debug library skeleton."
    dependsOn(":apps:android:assembleDebug")
}

tasks.register("workspaceTest") {
    group = "verification"
    description = "Runs the Android debug unit tests."
    dependsOn(":apps:android:testDebugUnitTest")
}
