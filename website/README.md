# VanillaPay Website

VanillaPay Website is the server-side application for merchant registration,
payment QR code management, Android monitor device access, order matching,
gateway-compatible order creation, asynchronous notifications, and operations
management.

The project is built on ThinkPHP 8, MySQL-compatible storage, Composer, and a
Tailwind CSS asset pipeline.

## Features

- Merchant registration, login, dashboard, orders, QR codes, and device binding.
- One monitor device per merchant, with heartbeat, configuration pull, and
  payment notification upload APIs.
- WeChat Pay and Alipay QR code management with server-side image processing.
- Floating amount matching and amount-lock release for expired orders.
- Gateway-compatible endpoints for EPay, CodePay, and YuanPay style clients.
- Downstream `notify_url` callback retry and daily reconciliation commands.
- `/console` operations console for merchants, devices, orders, channels, risk
  events, settings, and reconciliation.

## Architecture

```text
app/        Application modules, controllers, services, commands, middleware
config/     ThinkPHP and application configuration
database/   Migrations and seed-related database files
public/     Web document root and static assets
route/      HTTP route definitions
view/       Server-rendered templates
```

Important runtime paths:

```text
runtime/                 Framework cache, logs, and temporary files
public/static/uploads/   Uploaded QR code images and generated assets
```

These runtime paths must be writable in production and should not be committed.

## Requirements

- PHP 8.1 or newer. Use the same PHP CLI version as the web runtime.
- MySQL 8.x or another compatible MySQL database.
- Composer 2.x.
- Node.js 18 or newer, only required when rebuilding Tailwind CSS assets.
- PHP extensions: `pdo_mysql`, `mysqli`, `mbstring`, `openssl`, `curl`,
  `fileinfo`, `gd`, `zip`, and `opcache`.

## Configuration

Create the runtime environment file from the example:

```bash
cp .example.env .env
```

Use project-specific values in `.env`. Do not commit `.env`.

```ini
APP_DEBUG = false
APP_ENV = production
APP_KEY = replace-with-a-long-random-secret

DB_DRIVER = mysql
DB_TYPE = mysql
DB_HOST = your_db_host
DB_NAME = your_database
DB_USER = your_db_user
DB_PASS = your_secure_password
DB_PORT = 3306
DB_CHARSET = utf8mb4
DB_PREFIX = vp_

DEFAULT_LANG = zh-cn
```

`DB_PREFIX` is applied automatically by the framework. Migration files should
use logical table names and should not manually add the prefix.

## Local Development

Install dependencies:

```bash
composer install
npm install
```

Build Tailwind CSS:

```bash
npm run build:css
```

Run the development server:

```bash
php think run -p 8080
```

Run database migrations:

```bash
php think migrate:run
```

Run tests:

```bash
vendor/bin/phpunit
```

PHPUnit may require additional PHP extensions such as `dom`, `xml`, and
`xmlwriter`.

## Deployment Package

On Windows, create a production deployment archive:

```bat
cd website
pack-deploy.bat
```

The archive is written to:

```text
website/deploy/vanillapay-website-YYYYMMDD-HHMMSS.zip
```

The package includes the application code, routes, views, public assets,
Composer lock file, environment example, and deployment scripts. It excludes
local/runtime-only files such as:

```text
.env
vendor/
node_modules/
runtime/
tests/
deploy/
```

## Production Deployment

1. Upload and extract the deployment archive to the target site directory.
2. Set the web document root to the `public` directory.
3. Create `.env` from `.example.env` and fill in production values.
4. Ensure `runtime` and `public/static/uploads` are writable by the web server.
5. Run the deployment script:

```bash
cd /path/to/site
bash deploy-server.sh
```

If the server has multiple PHP or Composer installations, provide explicit
paths:

```bash
cd /path/to/site
PHP_BIN=/path/to/php COMPOSER_BIN=/path/to/composer bash deploy-server.sh
```

The script installs Composer dependencies, runs database migrations, clears
framework cache, validates route registration, and adjusts runtime permissions
when a standard web-server user is available.

Recommended Nginx rewrite rule:

```nginx
location / {
    if (!-e $request_filename) {
        rewrite ^(.*)$ /index.php?s=/$1 last;
        break;
    }
}

location ~ ^/static/uploads/.*\.php$ {
    deny all;
}
```

## Updating An Existing Deployment

For routine updates:

1. Create a new deployment archive.
2. Upload and extract it over the existing site files.
3. Keep the existing `.env`, `runtime`, `public/static/uploads`, and database.
4. Run:

```bash
cd /path/to/site
bash deploy-server.sh
```

`vendor`, `node_modules`, and `.env` are intentionally not included in the
deployment archive.

## Console Admin

Create the first operations-console administrator after the initial deployment:

```bash
php think vanilla:admin-create admin_user strong_password
```

After creation, sign in at:

```text
/console/login
```

Only create the initial administrator once. Routine deployments do not require
this command.

## Scheduled Tasks

Configure these commands to run once per minute:

```bash
/path/to/php /path/to/site/think vanilla:order-expire
/path/to/php /path/to/site/think vanilla:device-check
/path/to/php /path/to/site/think vanilla:callback-retry
```

Optional daily reconciliation:

```bash
/path/to/php /path/to/site/think vanilla:reconcile-daily
```

These tasks expire timed-out orders, release floating amount locks, mark stale
devices offline, retry downstream callbacks, and print reconciliation summaries.

## Merchant Flow

1. Register and sign in through `/register` and `/login`.
2. Upload one WeChat Pay QR code and one Alipay QR code in `/qrcodes`.
3. Bind the Android monitor device in `/devices`.
4. Keep the Android app running with notification access enabled.
5. Create orders through a supported gateway endpoint.
6. The system matches payment notifications and sends downstream callbacks.

The Android binding string uses this format:

```text
serverUrl|deviceId|deviceKey
```

## Gateway Endpoints

Supported endpoint groups:

```text
EPay:    /submit.php, /mapi.php, /api.php
CodePay: /creat_order/
YuanPay: /yuanpay/submit, /yuanpay/mapi
```

Use the public domain configured for your deployment when integrating external
clients, for example:

```text
https://your-domain.example/submit.php
```

## Device APIs

The Android monitor communicates with:

```text
POST /app/heart
POST /app/push
GET  /app/config
```

These endpoints are rate-limited and expect the device credentials generated by
the merchant device-binding page.

## Verification

After deployment, verify the application with placeholder-safe commands:

```bash
php think route:list
curl -I https://your-domain.example/login
curl https://your-domain.example/app/config
```

Then validate the full flow with a small test order:

1. Upload payment QR codes.
2. Bind the Android monitor.
3. Create an order through a gateway endpoint.
4. Confirm that the order moves from pending to paid or expired correctly.
5. Confirm that the downstream callback is delivered or queued for retry.

## Troubleshooting

### Composer reports `putenv() has been disabled`

The PHP CLI used by Composer may not match the PHP runtime used by the site.
Run Composer with the intended PHP binary and check the CLI `disable_functions`
configuration.

```bash
/path/to/php /path/to/composer install --no-dev --optimize-autoloader
```

### `zip.so` cannot be loaded

The PHP configuration references the zip extension, but the extension file is
missing or was installed for another PHP version. Install the zip extension for
the active PHP runtime, or remove the invalid `extension=zip.so` entry and
restart PHP-FPM.

### `There are no commands defined in the migrate namespace`

Composer dependencies are usually incomplete. Reinstall dependencies and confirm
that `topthink/think-migration` is present:

```bash
/path/to/php /path/to/composer install --no-dev --optimize-autoloader
```

### `Duplicate migration`

Migration versions must be unique. Check `database/migrations` for files with
the same timestamp version, keep a single valid migration for that version, then
rerun migrations.

### Existing tables but missing migration records

If a legacy database already contains some business tables but has no matching
migration records, back up the database first. Then run the baseline helper once
before the normal deployment script:

```bash
cd /path/to/site
PHP_BIN=/path/to/php bash deploy-baseline-existing-db.sh
bash deploy-server.sh
```

The baseline helper records migrations only for the continuous set of existing
tables from the initial schema. It does not delete data.

### Uploaded QR code image returns 404

Confirm that the web document root is `public` and that uploads are readable:

```bash
chmod -R 755 public/static/uploads
```

### QR code upload does not change the page

Check that `gd` and `fileinfo` are enabled, and that the upload directory is
writable. Upload errors are written to the framework logs under `runtime/log`.

## Security Notes

- Do not commit `.env`, database dumps, private keys, upload files, or runtime
  logs.
- Use HTTPS in production.
- Keep PHP, Composer packages, and server packages updated.
- Restrict write permissions to the minimum required runtime directories.
- Block execution of uploaded PHP files under `public/static/uploads`.
- Store gateway secrets and signing keys only in environment-specific
  configuration.
