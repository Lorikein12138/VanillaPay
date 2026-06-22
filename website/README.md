# VanillaPay 网站端

VanillaPay 网站端是服务端应用，负责商户注册、收款二维码管理、Android 监控设备接入、订单匹配、兼容网关的订单创建、异步通知和运营管理。

项目基于 ThinkPHP 8、MySQL 兼容存储、Composer 和 Tailwind CSS 资源构建流程。

## 功能

- 商户注册、登录、仪表盘、订单、二维码和设备绑定。
- 每个商户对应一台监控设备，提供心跳、配置拉取和支付通知上传 API。
- 微信支付和支付宝二维码管理，并在服务端进行图片处理。
- 浮动金额匹配，以及过期订单的金额锁释放。
- 面向 EPay、CodePay 和 YuanPay 风格客户端的兼容网关接口。
- 下游 `notify_url` 回调重试和每日对账命令。
- `/console` 运营控制台，用于管理商户、设备、订单、通道、风险事件、设置和对账。

## 架构

```text
app/        应用模块、控制器、服务、命令、中间件
config/     ThinkPHP 和应用配置
database/   迁移和种子相关数据库文件
public/     Web 文档根目录和静态资源
route/      HTTP 路由定义
view/       服务端渲染模板
```

重要运行时路径：

```text
runtime/                 框架缓存、日志和临时文件
public/static/uploads/   上传的二维码图片和生成的资源
```

这些运行时路径在生产环境必须可写，且不应提交到仓库。

## 环境要求

- PHP 8.1 或更高版本。PHP CLI 版本应与 Web 运行时保持一致。
- MySQL 8.x 或其他兼容 MySQL 的数据库。
- Composer 2.x。
- Node.js 18 或更高版本，仅在重新构建 Tailwind CSS 资源时需要。
- PHP 扩展：`pdo_mysql`、`mysqli`、`mbstring`、`openssl`、`curl`、`fileinfo`、`gd`、`zip` 和 `opcache`。

## 配置

从示例文件创建运行时环境文件：

```bash
cp .example.env .env
```

在 `.env` 中填写项目专用值。不要提交 `.env`。

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

`DB_PREFIX` 会由框架自动应用。迁移文件应使用逻辑表名，不要手动添加表前缀。

## 本地开发

安装依赖：

```bash
composer install
npm install
```

构建 Tailwind CSS：

```bash
npm run build:css
```

启动开发服务器：

```bash
php think run -p 8080
```

运行数据库迁移：

```bash
php think migrate:run
```

运行测试：

```bash
vendor/bin/phpunit
```

PHPUnit 可能需要额外的 PHP 扩展，例如 `dom`、`xml` 和 `xmlwriter`。

## 部署包

在 Windows 上创建生产部署压缩包：

```bat
cd website
pack-deploy.bat
```

压缩包会写入：

```text
website/deploy/vanillapay-website-YYYYMMDD-HHMMSS.zip
```

部署包包含应用代码、路由、视图、公共资源、Composer 锁文件、环境示例和部署脚本。它会排除本地或仅运行时使用的文件，例如：

```text
.env
vendor/
node_modules/
runtime/
tests/
deploy/
```

## 生产部署

1. 上传部署压缩包并解压到目标站点目录。
2. 将 Web 文档根目录设置为 `public` 目录。
3. 从 `.example.env` 创建 `.env`，并填写生产环境值。
4. 确保 `runtime` 和 `public/static/uploads` 可由 Web 服务器写入。
5. 运行部署脚本：

```bash
cd /path/to/site
bash deploy-server.sh
```

如果服务器上有多个 PHP 或 Composer 安装，请显式指定路径：

```bash
cd /path/to/site
PHP_BIN=/path/to/php COMPOSER_BIN=/path/to/composer bash deploy-server.sh
```

该脚本会安装 Composer 依赖、运行数据库迁移、清理框架缓存、验证路由注册，并在存在标准 Web 服务器用户时调整运行时权限。

推荐的 Nginx 重写规则：

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

## 更新已有部署

常规更新流程：

1. 创建新的部署压缩包。
2. 上传并解压覆盖现有站点文件。
3. 保留现有的 `.env`、`runtime`、`public/static/uploads` 和数据库。
4. 运行：

```bash
cd /path/to/site
bash deploy-server.sh
```

`vendor`、`node_modules` 和 `.env` 会被有意排除在部署压缩包之外。

## 控制台管理员

首次部署后，创建第一个运营控制台管理员：

```bash
php think vanilla:admin-create admin_user strong_password
```

创建后访问以下地址登录：

```text
/console/login
```

初始管理员只需创建一次。常规部署不需要执行此命令。

## 计划任务

配置以下命令每分钟运行一次：

```bash
/path/to/php /path/to/site/think vanilla:order-expire
/path/to/php /path/to/site/think vanilla:device-check
/path/to/php /path/to/site/think vanilla:callback-retry
```

可选的每日对账：

```bash
/path/to/php /path/to/site/think vanilla:reconcile-daily
```

这些任务会处理超时订单过期、释放浮动金额锁、将长时间无心跳设备标记为离线、重试下游回调，并输出对账摘要。

## 商户流程

1. 通过 `/register` 和 `/login` 注册并登录。
2. 在 `/qrcodes` 上传一个微信支付二维码和一个支付宝二维码。
3. 在 `/devices` 绑定 Android 监控设备。
4. 保持 Android 应用运行，并启用通知访问权限。
5. 通过受支持的网关接口创建订单。
6. 系统匹配支付通知并发送下游回调。

Android 绑定字符串格式如下：

```text
serverUrl|deviceId|deviceKey
```

## 网关接口

支持的接口组：

```text
EPay:    /submit.php, /mapi.php, /api.php
CodePay: /creat_order/
YuanPay: /yuanpay/submit, /yuanpay/mapi
```

集成外部客户端时，请使用部署配置的公网域名，例如：

```text
https://your-domain.example/submit.php
```

## 设备 API

Android 监控端与以下接口通信：

```text
POST /app/heart
POST /app/push
GET  /app/config?device_id=...&t=...&sign=...
```

这些接口带有频率限制，并要求使用商户设备绑定页面生成的设备凭据。

## 验证

部署后，使用不含真实敏感值的命令验证应用：

```bash
php think route:list
curl -I https://your-domain.example/login
curl https://your-domain.example/app/config
```

然后使用小额测试订单验证完整流程：

1. 上传收款二维码。
2. 绑定 Android 监控端。
3. 通过网关接口创建订单。
4. 确认订单能正确从待支付变为已支付或已过期。
5. 确认下游回调已投递，或已进入重试队列。

## 故障排查

### Composer reports `putenv() has been disabled`

Composer 使用的 PHP CLI 可能与站点运行时使用的 PHP 不一致。请使用目标 PHP 二进制运行 Composer，并检查 CLI 的 `disable_functions` 配置。

```bash
/path/to/php /path/to/composer install --no-dev --optimize-autoloader
```

### `zip.so` cannot be loaded

PHP 配置引用了 zip 扩展，但扩展文件缺失，或安装给了另一个 PHP 版本。请为当前 PHP 运行时安装 zip 扩展，或移除无效的 `extension=zip.so` 配置并重启 PHP-FPM。

### `There are no commands defined in the migrate namespace`

通常是 Composer 依赖不完整。请重新安装依赖，并确认 `topthink/think-migration` 已存在：

```bash
/path/to/php /path/to/composer install --no-dev --optimize-autoloader
```

### `Duplicate migration`

迁移版本必须唯一。检查 `database/migrations` 中是否存在相同时间戳版本的文件，为该版本保留一个有效迁移文件，然后重新运行迁移。

### 已有数据表但缺少迁移记录

如果旧数据库已经包含部分业务表，但没有对应迁移记录，请先备份数据库。然后在正常部署脚本之前运行一次基线辅助脚本：

```bash
cd /path/to/site
PHP_BIN=/path/to/php bash deploy-baseline-existing-db.sh
bash deploy-server.sh
```

基线辅助脚本只会为初始架构中连续存在的数据表记录迁移，不会删除数据。

### 上传的二维码图片返回 404

确认 Web 文档根目录为 `public`，并且上传文件可读：

```bash
chmod -R 755 public/static/uploads
```

### 上传二维码后页面没有变化

检查是否已启用 `gd` 和 `fileinfo`，并确认上传目录可写。上传错误会写入 `runtime/log` 下的框架日志。

## 安全说明

- 不要提交 `.env`、数据库转储、私钥、上传文件或运行时日志。
- 生产环境使用 HTTPS。
- 保持 PHP、Composer 包和服务器软件包及时更新。
- 将写权限限制在最低必要的运行时目录。
- 禁止执行 `public/static/uploads` 下上传的 PHP 文件。
- 网关密钥和签名密钥只应存储在按环境区分的配置中。
