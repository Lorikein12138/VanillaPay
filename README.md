# VanillaPay

VanillaPay is a self-hosted payment order and notification matching system for
merchant-owned deployments. It combines a ThinkPHP website with an Android
monitoring client to manage payment QR codes, create gateway-compatible orders,
match incoming payment notifications, and deliver downstream callbacks.

## Repository Layout

```text
website/   ThinkPHP 8 web application, merchant console, device APIs, gateways
android/   Android monitoring client for WeChat Pay and Alipay notifications
```

## Components

### Website

The website provides merchant registration, QR code management, order creation,
floating amount matching, device binding, callback retry, reconciliation tasks,
and an operations console.

Main stack:

- PHP 8.1 or newer with ThinkPHP 8
- MySQL-compatible database
- Composer
- Tailwind CSS asset pipeline

See [website/README.md](website/README.md) for configuration, deployment,
scheduled tasks, gateway endpoints, and troubleshooting.

### Android Monitor

The Android client runs on a merchant-owned device, listens for payment arrival
notifications, queues reports while offline, sends heartbeats, and syncs parsing
rules from the website.

Main stack:

- JDK 21
- Android SDK Platform 36
- Android Gradle Plugin 8.10.0
- Gradle Wrapper 8.11.1
- Android API 24 or newer device/emulator

See [android/README.md](android/README.md) for build, binding, permission,
release signing, and integration details.

## Development

Clone the repository and work inside the component you need:

```bash
git clone <repository-url>
cd VanillaPay
```

Website:

```bash
cd website
composer install
npm install
cp .example.env .env
npm run build:css
php think migrate:run
php think run -p 8080
```

Android:

```powershell
cd android
.\gradlew.bat :app:testDebugUnitTest
.\gradlew.bat :app:assembleDebug
```

## Production Notes

- Deploy the website with the document root set to `website/public`.
- Keep `website/.env`, runtime logs, uploads, database dumps, keystores, and
  signing files out of Git.
- Use HTTPS in production. The Android client disables cleartext HTTP by
  default.
- Configure scheduled website tasks for order expiry, device checks, callback
  retry, and optional daily reconciliation.
- Test the complete flow before production: merchant registration, QR upload,
  Android binding, order creation, notification matching, and downstream
  callback delivery.

## Security

VanillaPay handles payment-related credentials, device credentials, uploaded QR
codes, and callback secrets. Use strong environment-specific secrets, restrict
server write permissions, block execution from upload directories, rotate device
credentials when devices are replaced, and keep dependencies updated.

Operate the system only with accounts, devices, and payment channels you are
authorized to use, and follow the requirements of your payment providers and
local regulations.
