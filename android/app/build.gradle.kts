import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.ksp)
}

val releaseSigningProperties = Properties()
val releaseSigningPropertiesFile = rootProject.file("signing.properties")
if (releaseSigningPropertiesFile.isFile) {
    releaseSigningPropertiesFile.inputStream().use(releaseSigningProperties::load)
}

fun releaseSigningValue(propertyName: String, envName: String): String? =
    System.getenv(envName)?.takeIf { it.isNotBlank() }
        ?: releaseSigningProperties.getProperty(propertyName)?.takeIf { it.isNotBlank() }

android {
    namespace = "com.vanillapay.monitor"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.vanillapay.monitor"
        minSdk = 24
        targetSdk = 36
        versionCode = 9
        versionName = "1.1.7"
        buildConfigField("String", "CERT_PIN_HOST", "\"\"")
        buildConfigField("String", "CERT_PIN_SHA256", "\"\"")
    }

    buildFeatures {
        buildConfig = true
    }

    signingConfigs {
        create("release") {
            storeFile = rootProject.file(releaseSigningValue("storeFile", "VP_KEYSTORE") ?: "release.keystore")
            storePassword = releaseSigningValue("storePassword", "VP_STORE_PWD") ?: ""
            keyAlias = releaseSigningValue("keyAlias", "VP_KEY_ALIAS") ?: "vanillapay"
            keyPassword = releaseSigningValue("keyPassword", "VP_KEY_PWD") ?: ""
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
    implementation(libs.material)
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
    testRuntimeOnly(libs.junit.platform.launcher)
}
