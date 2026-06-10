# VanillaPay Monitor Android

VanillaPay Monitor Android is the mobile monitoring client for merchant-owned
devices. It listens for WeChat Pay and Alipay payment notifications, extracts
payment amounts, queues reports while offline, sends device heartbeats, and
syncs parsing rules from the website.

## Features

- Device binding through a QR code or pasted binding string.
- Notification listener for payment arrival messages.
- Local offline queue with retry after network recovery.
- Heartbeat reporting to keep the website device status online.
- Server-side rule synchronization through `/app/config`.
- Optional HTTPS certificate pinning through Gradle build configuration.

## Requirements

- JDK 21.
- Android SDK Platform 36.
- Android Gradle Plugin 8.10.0.
- Gradle Wrapper 8.11.1.
- Android device or emulator running API 24 or newer.

## Project Structure

```text
app/src/main/     Application source code and Android resources
app/src/test/     JVM unit tests
gradle/           Version catalog and Gradle Wrapper metadata
```

The app package name is defined in `app/build.gradle.kts`.

## Configuration

Create `local.properties` in the Android project directory and point it to the
local Android SDK:

```properties
sdk.dir=/path/to/android/sdk
```

On Windows, a path can use escaped backslashes or forward slashes:

```properties
sdk.dir=D:/Android/Sdk
```

`local.properties` is intentionally ignored by Git.

## Build Commands

Run unit tests:

```powershell
.\gradlew.bat :app:testDebugUnitTest
```

Build a debug APK:

```powershell
.\gradlew.bat :app:assembleDebug
```

Install the debug APK on a connected device:

```powershell
.\gradlew.bat :app:installDebug
```

Debug APK output:

```text
app/build/outputs/apk/debug/app-debug.apk
```

## Device Binding

Generate a device binding string in the website merchant device page. The
binding payload uses this format:

```text
serverUrl|deviceId|deviceKey
```

The app accepts either a scanned QR code or a pasted binding string. The server
URL should be the public HTTPS address of the website deployment.

## Runtime Permissions

After binding, configure the device:

1. Grant notification listener access to the app.
2. Allow foreground notifications.
3. Allow background execution or startup management where the device vendor
   requires it.
4. Disable battery optimization for the app when required for stable listening.
5. Return to the app and confirm that the listener status is running.

Vendor-specific battery and background restrictions should be tested on the
actual production device model.

## Network Security

Cleartext HTTP is disabled by default. Use HTTPS for production.

Certificate pinning can be configured with these build fields:

```text
BuildConfig.CERT_PIN_HOST
BuildConfig.CERT_PIN_SHA256
```

When no certificate pin is configured, the app uses the platform HTTPS trust
store.

## Release Build

Release signing reads credentials from `signing.properties` in the Android
project root. This file is ignored by Git and should be kept with the release
keystore.

```properties
storeFile=release.keystore
storePassword=replace-with-store-password
keyAlias=release-key-alias
keyPassword=replace-with-key-password
```

Environment variables can override the local file when needed:

```powershell
$env:VP_KEYSTORE="D:\path\to\release.keystore"
$env:VP_STORE_PWD="store-password"
$env:VP_KEY_ALIAS="release-key-alias"
$env:VP_KEY_PWD="key-password"
```

Build the release APK:

```powershell
.\gradlew.bat :app:assembleRelease
```

Release APK output:

```text
app/build/outputs/apk/release/app-release.apk
```

Do not commit keystores, signing passwords, or generated release artifacts.

## Integration Checklist

Validate the app with the website before production use:

- Binding succeeds by QR code and by pasted binding string.
- `/app/heart` keeps the website device status online.
- `/app/config` updates parsing rules without rebuilding the APK.
- WeChat Pay and Alipay payment notifications are parsed and uploaded to
  `/app/push`.
- Offline reports remain queued and are sent after network recovery.
- Repeated uploads do not cause duplicate order settlement.
- Long-running monitoring works after screen-off, app backgrounding, and network
  changes.

## Testing

Run the JVM unit test suite:

```powershell
.\gradlew.bat :app:testDebugUnitTest
```

Run a debug build before shipping changes:

```powershell
.\gradlew.bat :app:assembleDebug
```

Real-device testing is required because notification listener behavior and
background execution policies vary by Android vendor.

## Operational Notes

- Keep the device online and connected to a stable network.
- Use one merchant-owned device per merchant account.
- Keep website endpoint, device ID, and device key out of public logs.
- Rotate device credentials from the website if a device is lost or replaced.
- Keep the APK and website API versions aligned during deployment.
