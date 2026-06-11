# VanillaPay

VanillaPay 是一个面向商户自有部署的自托管支付订单与通知匹配系统。它结合 ThinkPHP 网站与 Android 监控客户端，用于管理收款二维码、创建兼容网关的订单、匹配到账通知，并向下游发送回调。

## 仓库结构

```text
website/   ThinkPHP 8 Web 应用、商户控制台、设备 API、支付网关
android/   用于监听微信支付和支付宝通知的 Android 监控客户端
```

## 组件

### 网站端

网站端提供商户注册、二维码管理、订单创建、浮动金额匹配、设备绑定、回调重试、对账任务和运营控制台。

主要技术栈：

- PHP 8.1 或更高版本，基于 ThinkPHP 8
- MySQL 兼容数据库
- Composer
- Tailwind CSS 资源构建流程

配置、部署、计划任务、网关接口和故障排查请参见 [website/README.md](website/README.md)。

### Android 监控端

Android 客户端运行在商户自有设备上，用于监听支付到账通知，在离线时缓存上报，发送设备心跳，并从网站端同步解析规则。

主要技术栈：

- JDK 21
- Android SDK Platform 36
- Android Gradle Plugin 8.10.0
- Gradle Wrapper 8.11.1
- Android API 24 或更高版本的设备/模拟器

构建、绑定、权限、发布签名和集成细节请参见 [android/README.md](android/README.md)。

## 开发

克隆仓库后进入需要开发的组件目录：

```bash
git clone <repository-url>
cd VanillaPay
```

网站端：

```bash
cd website
composer install
npm install
cp .example.env .env
npm run build:css
php think migrate:run
php think run -p 8080
```

Android 端：

```powershell
cd android
.\gradlew.bat :app:testDebugUnitTest
.\gradlew.bat :app:assembleDebug
```

## 生产环境说明

- 部署网站端时，将 Web 文档根目录设置为 `website/public`。
- 不要将 `website/.env`、运行日志、上传文件、数据库转储、密钥库和签名文件提交到 Git。
- 生产环境使用 HTTPS。Android 客户端默认禁用明文 HTTP。
- 配置网站端计划任务，用于订单过期、设备检查、回调重试，以及可选的每日对账。
- 上线前测试完整流程：商户注册、二维码上传、Android 绑定、订单创建、通知匹配和下游回调投递。

## 安全

VanillaPay 会处理支付相关凭据、设备凭据、上传的二维码和回调密钥。请使用强度足够且按环境区分的密钥，限制服务器写权限，禁止上传目录中的脚本执行，在更换设备时轮换设备凭据，并保持依赖及时更新。

仅在你有权使用的账号、设备和支付通道上运行本系统，并遵守支付服务提供方要求及当地法规。
