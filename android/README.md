# VanillaPay Monitor Android

VanillaPay 安卓监听端用于商户本人手机的微信/支付宝到账通知监听、金额解析、离线上报队列、心跳和设备绑定。

## 环境

- JDK 21
- Android SDK Platform 35
- Gradle 8.10 Wrapper

首次构建前确认 `local.properties` 指向本机 SDK：

```properties
sdk.dir=C:/Users/Lorikein/AppData/Local/Android/Sdk
```

`local.properties` 已被 `.gitignore` 忽略，不需要提交。

## 常用命令

```powershell
.\gradlew.bat :app:testDebugUnitTest
.\gradlew.bat :app:assembleDebug
.\gradlew.bat :app:installDebug
```

Debug APK 输出：

```text
app/build/outputs/apk/debug/app-debug.apk
```

## 绑定与运行

1. 在网站端生成设备绑定串，格式为 `serverUrl|deviceId|deviceKey`。
2. 打开 App，粘贴绑定串或扫码绑定。
3. 点击 `开启通知使用权`，在系统设置中授予 VanillaPay 通知使用权。
4. 在系统中允许前台通知、自启动、忽略电池优化。
5. 回到主页确认显示 `监听运行中`。

## 联调检查

```powershell
.\gradlew.bat :app:testDebugUnitTest
.\gradlew.bat :app:assembleDebug
```

真机上需要验证：

- 收到微信/支付宝到账通知后，记录入队并上报 `/app/push`。
- 断网后记录保留，恢复网络后补发。
- `/app/heart` 心跳使网站端设备保持在线。
- `/app/config` 规则下发后，无需发版即可更新解析规则。

## 网络安全与证书绑定

默认禁用明文 HTTP。生产环境请使用 HTTPS。

证书 pin 可通过 `BuildConfig.CERT_PIN_HOST`、`BuildConfig.CERT_PIN_SHA256` 配置。未配置 pin 时仅启用系统 HTTPS 校验。

## Release 打包

Release 签名从环境变量读取：

```powershell
$env:VP_KEYSTORE="D:\path\vanillapay-release.keystore"
$env:VP_STORE_PWD="store-password"
$env:VP_KEY_ALIAS="vanillapay"
$env:VP_KEY_PWD="key-password"
.\gradlew.bat :app:assembleRelease
```

Release APK 输出：

```text
app/build/outputs/apk/release/app-release.apk
```

## 实测清单

- Pixel/原生 Android：息屏、清后台、断网恢复。
- 小米/HyperOS：自启动、后台无限制、电池白名单。
- 华为/鸿蒙：应用启动管理手动管理、允许后台。
- OPPO/vivo：自启动、后台高耗电白名单。
- 三星 OneUI：避免进入休眠应用。

通过标准：网站设备状态持续在线；到账通知能上报；断网恢复后不丢单、不重复核销。
