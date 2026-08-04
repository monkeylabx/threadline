plugins {
    base
}

val kotlinMain = layout.projectDirectory.file(
    "src/main/kotlin/com/threadline/android/ThreadlineAndroidSkeleton.kt"
)
val kotlinTest = layout.projectDirectory.file(
    "src/test/kotlin/com/threadline/android/ThreadlineAndroidSkeletonTest.kt"
)
val kotlinOutput = layout.buildDirectory.dir("kotlin-skeleton")

val buildKotlinSkeleton by tasks.registering(Exec::class) {
    group = "build"
    description = "Compiles the dependency-free Kotlin host skeleton."
    inputs.file(kotlinMain)
    outputs.file(kotlinOutput.map { it.file("skeleton.jar") })
    doFirst { kotlinOutput.get().asFile.mkdirs() }
    commandLine(
        "kotlinc",
        kotlinMain.asFile.absolutePath,
        "-d",
        kotlinOutput.get().file("skeleton.jar").asFile.absolutePath,
    )
}

val compileKotlinSkeletonTest by tasks.registering(Exec::class) {
    group = "verification"
    description = "Compiles the dependency-free Kotlin host test."
    inputs.files(kotlinMain, kotlinTest)
    outputs.file(kotlinOutput.map { it.file("skeleton-test.jar") })
    doFirst { kotlinOutput.get().asFile.mkdirs() }
    commandLine(
        "kotlinc",
        kotlinMain.asFile.absolutePath,
        kotlinTest.asFile.absolutePath,
        "-include-runtime",
        "-d",
        kotlinOutput.get().file("skeleton-test.jar").asFile.absolutePath,
    )
}

val testKotlinSkeleton by tasks.registering(Exec::class) {
    group = "verification"
    description = "Runs the dependency-free Kotlin host test."
    dependsOn(compileKotlinSkeletonTest)
    commandLine(
        "java",
        "-jar",
        kotlinOutput.get().file("skeleton-test.jar").asFile.absolutePath,
    )
}

tasks.named("assemble") { dependsOn(buildKotlinSkeleton) }
tasks.named("check") { dependsOn(testKotlinSkeleton) }
