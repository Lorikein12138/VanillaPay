# VanillaPay Android 监控端

VanillaPay Android 监控端是运行在商户自有设备上的移动监控客户端。它监听微信支付和支付宝的到账通知，提取支付金额，在离线时缓存上报，发送设备心跳，并从网站端同步解析规则。

## 功能

- 通过二维码或粘贴绑定字符串完成设备绑定。
- 通过通知监听服务监听支付到账消息。
- 本地离线队列，在网络恢复后自动重试上报。
- 心跳上报，用于保持网站端设备状态在线。
- 通过 `/app/config` 同步服务端规则。
- 可通过 Gradle 构建配置启用 HTTPS 证书锁定。

## 环境要求

- JDK 21。
- Android SDK Platform 36。
- Android Gradle Plugin 8.10.0。
- Gradle Wrapper 8.11.1。
- 运行 API 24 或更高版本的 Android 设备或模拟器。

## 项目结构

```text
app/src/main/     应用源代码和 Android 资源
app/src/test/     JVM 单元测试
gradle/           版本目录和 Gradle Wrapper 元数据
```

应用包名定义在 `app/build.gradle.kts` 中。

## 配置

在 Android 项目目录中创建 `local.properties`，并指向本地 Android SDK：

```properties
sdk.dir=/path/to/android/sdk
```

在 Windows 上，路径可以使用转义反斜杠或正斜杠：

```properties
sdk.dir=D:/Android/Sdk
```

`local.properties` 已被有意加入 Git 忽略。

## 构建命令

运行单元测试：

```powershell
.\gradlew.bat :app:testDebugUnitTest
```

构建调试 APK：

```powershell
.\gradlew.bat :app:assembleDebug
```

将调试 APK 安装到已连接设备：

```powershell
.\gradlew.bat :app:installDebug
```

调试 APK 输出位置：

```text
app/build/outputs/apk/debug/app-debug.apk
```

## 设备绑定

在网站端商户设备页面生成设备绑定字符串。绑定数据格式如下：

```text
serverUrl|deviceId|deviceKey
```

应用支持扫描二维码或粘贴绑定字符串。服务器 URL 应使用网站部署后的公网 HTTPS 地址。

## 运行时权限

绑定后，按以下步骤配置设备：

1. 授予应用通知监听权限。
2. 允许前台通知。
3. 如果设备厂商要求，允许后台运行或开启自启动管理。
4. 为保证稳定监听，必要时关闭应用的电池优化。
5. 返回应用并确认监听状态正在运行。

不同厂商对电池和后台运行的限制不同，应在实际生产设备型号上进行测试。

## 网络安全

默认禁用明文 HTTP。生产环境请使用 HTTPS。

证书锁定可通过以下构建字段配置：

```text
BuildConfig.CERT_PIN_HOST
BuildConfig.CERT_PIN_SHA256
```

未配置证书锁定时，应用使用系统平台的 HTTPS 信任存储。

## 发布构建

发布签名会从 Android 项目根目录的 `signing.properties` 读取凭据。该文件已被 Git 忽略，应与发布密钥库一起妥善保存。

```properties
storeFile=release.keystore
storePassword=replace-with-store-password
keyAlias=release-key-alias
keyPassword=replace-with-key-password
```

需要时，可使用环境变量覆盖本地文件：

```powershell
$env:VP_KEYSTORE="D:\path\to\release.keystore"
$env:VP_STORE_PWD="store-password"
$env:VP_KEY_ALIAS="release-key-alias"
$env:VP_KEY_PWD="key-password"
```

构建发布 APK：

```powershell
.\gradlew.bat :app:assembleRelease
```

发布 APK 输出位置：

```text
app/build/outputs/apk/release/app-release.apk
```

不要提交密钥库、签名密码或生成的发布产物。

## 集成检查清单

生产使用前，请配合网站端验证应用：

- 通过二维码和粘贴绑定字符串均可成功绑定。
- `/app/heart` 能保持网站端设备状态在线。
- `/app/config` 能在不重新构建 APK 的情况下更新解析规则。
- 微信支付和支付宝到账通知能够被解析并上传到 `/app/push`。
- 离线上报会保留在队列中，并在网络恢复后发送。
- 重复上传不会导致订单重复结算。
- 熄屏、应用退至后台和网络变化后，长时间监控仍能正常工作。

## 测试

运行 JVM 单元测试套件：

```powershell
.\gradlew.bat :app:testDebugUnitTest
```

发布变更前运行调试构建：

```powershell
.\gradlew.bat :app:assembleDebug
```

通知监听行为和后台运行策略会因 Android 厂商而异，因此必须进行真机测试。

## 运维说明

- 保持设备在线，并连接到稳定网络。
- 每个商户账号使用一台商户自有设备。
- 不要将网站端接口地址、设备 ID 和设备密钥写入公开日志。
- 如果设备丢失或更换，请从网站端轮换设备凭据。
- 部署时保持 APK 与网站端 API 版本匹配。
