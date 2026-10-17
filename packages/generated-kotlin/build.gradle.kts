import org.gradle.api.artifacts.dsl.LockMode

plugins {
    `java-library`
    kotlin("jvm") version "2.4.10"
}

kotlin { jvmToolchain(17) }

dependencyLocking {
    lockAllConfigurations()
    lockMode.set(LockMode.STRICT)
}

dependencies {
    api("com.google.protobuf:protobuf-java:4.35.1")
    api("com.google.protobuf:protobuf-kotlin:4.35.1")
    api("com.connectrpc:connect-kotlin:0.9.0")
    testImplementation(kotlin("test-junit"))
}
