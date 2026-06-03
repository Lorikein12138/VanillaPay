# VanillaPay Website

VanillaPay 网站端基于 ThinkPHP 8 + MySQL，已实现商户账户、收款码上传、安卓监控设备接入、金额浮动核销、易支付/码支付/源支付网关、异步回调、商户中心、`/console` 超管后台、限流、防重放、审计日志和对账。

## 生产环境要求

- 宝塔面板 + Nginx
- PHP 8.4/8.5，建议使用站点同版本命令，例如 `/www/server/php/85/bin/php`
- MySQL 8.4.8
- Composer
- PHP 扩展：`pdo_mysql`、`mysqli`、`mbstring`、`openssl`、`curl`、`fileinfo`、`zip`、`opcache`
- Node.js 仅用于重新构建 Tailwind；生产部署不需要上传 `node_modules`

## 本地打包

Windows 本地执行：

```bat
cd website
pack-deploy.bat
```

压缩包输出到 `website/deploy/vanillapay-website-YYYYMMDD-HHMMSS.zip`。压缩包会包含运行所需的 `app/config/database/public/route/view/think/composer.json/deploy-server.sh/README.md` 等文件，不包含：

```text
.env
vendor/
node_modules/
runtime/
tests/
deploy/
```

## 宝塔部署流程

以下示例以你的站点目录为准：

```bash
/www/wwwroot/vanillapay.lorikein.cn
```

1. 宝塔安装 Nginx、MySQL 8.4.8、PHP 8.5、Composer。
2. PHP 8.5 安装并启用上述扩展，确认命令行也是 8.5：

```bash
/www/server/php/85/bin/php -v
/www/server/php/85/bin/php -m | grep -E "pdo_mysql|mbstring|curl|fileinfo|zip"
```

3. 上传部署 zip 到站点目录并解压。
4. 首次部署时配置 `.env`：

```bash
cp .example.env .env
```

生产建议：

```ini
APP_DEBUG = false
APP_ENV = production
APP_KEY = 换成至少32位随机字符串

DB_DRIVER = mysql
DB_TYPE = mysql
DB_HOST = 127.0.0.1
DB_NAME = vanillapay
DB_USER = vanillapay
DB_PASS = 数据库强密码
DB_PORT = 3306
DB_CHARSET = utf8mb4
DB_PREFIX = vp_
```

5. 设置网站运行目录为 `/public`，默认文档包含 `index.php`。
6. Nginx 伪静态：

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

7. 执行一键部署脚本：

```bash
cd /www/wwwroot/vanillapay.lorikein.cn
bash deploy-server.sh
```

脚本会自动执行：

```bash
/www/server/php/85/bin/php /usr/bin/composer install --no-dev --optimize-autoloader
/www/server/php/85/bin/php think migrate:run
/www/server/php/85/bin/php think clear
chown -R www:www runtime public/static
chmod -R 755 runtime public/static
```

迁移会创建 `vp_users`、`vp_admins`、`vp_channels`、`vp_merchant_qrcodes`、`vp_devices`、`vp_orders`、`vp_order_amount_lock`、`vp_risk_events`、`vp_callback_logs`、`vp_settings`、`vp_login_logs`、`vp_operation_logs` 等表。

如果 PHP 或 Composer 路径和默认不同，可显式指定：

```bash
PHP_BIN=/www/server/php/85/bin/php COMPOSER_BIN=/usr/bin/composer bash deploy-server.sh
```

8. 创建超管：

```bash
/www/server/php/85/bin/php think vanilla:admin-create root 强密码
```

超管只需要首次创建，后续覆盖更新不要重复执行。

9. 宝塔计划任务，每分钟执行：

```bash
/www/server/php/85/bin/php /www/wwwroot/vanillapay.lorikein.cn/think vanilla:order-expire
/www/server/php/85/bin/php /www/wwwroot/vanillapay.lorikein.cn/think vanilla:device-check
/www/server/php/85/bin/php /www/wwwroot/vanillapay.lorikein.cn/think vanilla:callback-retry
```

每日对账可选：

```bash
/www/server/php/85/bin/php /www/wwwroot/vanillapay.lorikein.cn/think vanilla:reconcile-daily
```

## 后续覆盖更新

后续每次更新只需要：

1. 上传最新 `vanillapay-website-YYYYMMDD-HHMMSS.zip` 到站点目录。
2. 在宝塔文件管理中解压并覆盖同名文件。
3. 执行：

```bash
cd /www/wwwroot/vanillapay.lorikein.cn
bash deploy-server.sh
```

不要删除服务器上的 `.env`、`runtime`、`public/static/uploads` 和数据库。部署包默认不包含 `vendor`、`node_modules`、`.env`，所以覆盖同名文件即可。

## 使用流程

1. 访问 `/register` 注册商户，登录后进入 `/dashboard`。
2. 在 `/qrcodes` 上传微信/支付宝收款码。
3. 在 `/devices` 生成绑定串，格式为 `https://域名|device_id|device_key`，安卓 App 扫码或粘贴绑定。
4. 安卓 App 授权通知读取后，会调用：
   - `POST /app/heart`
   - `POST /app/push`
   - `GET /app/config`
5. 下游商户可通过网关下单：
   - 易支付：`/submit.php`、`/mapi.php`、`/api.php`
   - 码支付：`/creat_order/`
   - 源支付：`/yuanpay/submit`、`/yuanpay/mapi`
6. 订单支付成功后系统按订单协议回调 `notify_url`，失败由 `vanilla:callback-retry` 重试。
7. 超管访问 `/console/login`，管理商户、订单、设备、渠道、风控、设置和对账。

## 验证命令

```bash
/www/server/php/85/bin/php think route:list
curl -I https://vanillapay.lorikein.cn/login
curl https://vanillapay.lorikein.cn/app/config
```

本地开发：

```bash
composer install
npm install
npm run build:css
php think run -p 8080
```

测试：

```bash
vendor/bin/phpunit
```

PHPUnit 需要 `mbstring`、`dom`、`xml`、`xmlwriter` 等扩展。

## 常见问题

### composer 报 `putenv() has been disabled`

宝塔服务器可能同时安装多个 PHP 版本，命令行默认调用了旧版本。使用站点 PHP 完整路径执行：

```bash
/www/server/php/85/bin/php /usr/bin/composer install --no-dev --optimize-autoloader
```

### `zip.so` 加载失败

PHP 配置里启用了 zip 扩展但文件不存在。到宝塔 PHP 8.5 扩展管理安装 zip，或删除错误的 `extension=zip.so` 后重启 PHP。

### `There are no commands defined in the migrate namespace`

通常是 Composer 依赖没装完整，确认 `topthink/think-migration` 已安装：

```bash
/www/server/php/85/bin/php /usr/bin/composer install --no-dev --optimize-autoloader
```

### `Duplicate migration`

迁移文件版本号重复，检查 `database/migrations` 是否有同时间戳文件。保留唯一文件后重新执行。

### `Table 'vp_users' already exists` 或登录报缺少 `vp_login_logs`

这是旧库里已经有部分业务表，但 `vp_migrations` 迁移记录为空或缺失导致的状态。不要删除数据库，也不要重建 `vp_users`。

先用宝塔数据库备份当前库，或在服务器上备份：

```bash
mysqldump --single-transaction --quick --no-tablespaces -u数据库用户 -p vanillapay > /root/vanillapay.before-migrate.$(date +%Y%m%d%H%M%S).sql
```

然后执行一次性基线脚本，再跑常规部署：

```bash
cd /www/wwwroot/vanillapay.lorikein.cn
PHP_BIN=/www/server/php/85/bin/php bash deploy-baseline-existing-db.sh
bash deploy-server.sh
```

`deploy-baseline-existing-db.sh` 只会给从最早迁移开始、连续已经存在的旧表补迁移记录。后续缺失表仍由 `deploy-server.sh` 的 `migrate:run` 正常创建；如果脚本提示数据库不是连续基线状态，停止执行并先人工检查表结构。

### 数据表出现 `vp_vp_`

迁移文件不得手写 `vp_` 前缀。本项目新迁移都使用逻辑表名，由 `.env` 的 `DB_PREFIX=vp_` 自动处理。

### 上传二维码后图片 404

确认站点运行目录是 `/public`，并且 `public/static/uploads/qrcodes` 可读写：

```bash
chown -R www:www public/static/uploads
chmod -R 755 public/static/uploads
```
