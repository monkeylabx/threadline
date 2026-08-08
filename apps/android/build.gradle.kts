plugins {
    id("com.android.library")
}

android {
    namespace = "com.threadline.android"
    compileSdk = 37
    buildToolsVersion = "36.0.0"
    ndkVersion = "28.2.13676358"

    defaultConfig {
        minSdk = 28
        testInstrumentationRunner = "android.test.InstrumentationTestRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

tasks.withType<Test>().configureEach {
    val ffiLibraryDirectory = System.getenv("THREADLINE_FFI_LIBRARY_DIR")
        ?: error("THREADLINE_FFI_LIBRARY_DIR must point to the Rust cdylib")
    systemProperty("java.library.path", ffiLibraryDirectory)
}

dependencies {
    testImplementation("junit:junit:4.13.2")
}
