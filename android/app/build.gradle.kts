plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.ksp)
}

android {
    namespace = "com.vanillapay.monitor"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.vanillapay.monitor"
        minSdk = 24
        targetSdk = 35
        versionCode = 1
        versionName = "1.0.0"
        buildConfigField("String", "CERT_PIN_HOST", "\"\"")
        buildConfigField("String", "CERT_PIN_SHA256", "\"\"")
    }

    buildFeatures {
        buildConfig = true
    }

    signingConfigs {
        create("release") {
            storeFile = file(System.getenv("VP_KEYSTORE") ?: "release.keystore")
            storePassword = System.getenv("VP_STORE_PWD") ?: ""
            keyAlias = System.getenv("VP_KEY_ALIAS") ?: "vanillapay"
            keyPassword = System.getenv("VP_KEY_PWD") ?: ""
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            signingConfig = signingConfigs.getByName("release")
        }
    }
}

kotlin {
    jvmToolchain(21)
}

tasks.withType<Test> {
    useJUnitPlatform()
}

dependencies {
    implementation(libs.core.ktx)
    implementation(libs.appcompat)
    implementation(libs.lifecycle.runtime.ktx)
    implementation(libs.okhttp)
    implementation(libs.room.runtime)
    implementation(libs.room.ktx)
    ksp(libs.room.compiler)
    implementation(libs.security.crypto)
    implementation(libs.work.runtime)
    implementation(libs.coroutines)
    implementation(libs.camera.camera2)
    implementation(libs.camera.lifecycle)
    implementation(libs.camera.view)
    implementation(libs.mlkit.barcode)

    testImplementation(libs.junit.jupiter)
    testImplementation(libs.json)
}
