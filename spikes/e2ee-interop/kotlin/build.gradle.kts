plugins {
    kotlin("jvm") version "2.4.10"
    application
}

repositories {
    mavenCentral()
}

application {
    mainClass = "com.threadline.e2ee.MainKt"
}

tasks.named<JavaExec>("run") {
    standardInput = System.`in`
}
