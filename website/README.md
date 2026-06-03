# VanillaPay Website

VanillaPay 网站端基于 ThinkPHP 8，当前阶段已实现商户注册、登录、退出、找回密码、MySQL 持久化仓储、数据库迁移和本地 Tailwind CSS 构建。

## 生产环境要求

- 宝塔面板：Linux 面板，Web 服务建议 Nginx。
- PHP：建议 PHP 8.4 或 8.5；本项目当前开发验证环境为 PHP 8.5.4。
- MySQL：生产环境使用 MySQL 8.4.8。
- Composer：用于安装 PHP 依赖。
- Node.js：仅在服务器上需要重新构建 Tailwind CSS 时使用；如果直接部署已构建的 `public/static/dist/app.css`，生产环境可不安装 Node.js。

PHP 必需扩展：

- `pdo_mysql`
- `mysqli`
- `mbstring`
- `openssl`
- `curl`
- `fileinfo`
- `zip`
- `opcache`，建议生产开启

## 宝塔部署流程

以下示例假设项目部署到：

```bash
/www/wwwroot/vanillapay/website
```

### 1. 宝塔安装运行环境

在宝塔面板的软件商店安装：

- Nginx
- MySQL 8.4.8
- PHP 8.4 或 PHP 8.5
- Composer

在 PHP 设置中安装并启用上面列出的扩展，然后重载 PHP 服务。

### 2. 创建数据库

宝塔面板进入 `数据库`，创建数据库：

```text
数据库名：vanillapay
用户名：vanillapay
密码：使用强密码
编码：utf8mb4
```

记录数据库密码，稍后写入 `.env`。

### 3. 上传代码

方式一：Git 拉取。

```bash
cd /www/wwwroot
git clone <your-repo-url> vanillapay
cd /www/wwwroot/vanillapay/website
```

方式二：宝塔文件管理上传项目压缩包，解压后确保 `website` 目录下能看到：

```text
app/
config/
database/
public/
route/
think
composer.json
package.json
```

方式三：使用本地打包脚本生成部署压缩包。

Windows 本地双击或在终端执行：

```bat
pack-deploy.bat
```

压缩包会生成到：

```text
website/deploy/vanillapay-website-YYYYMMDD-HHMMSS.zip
```

该压缩包只包含部署运行需要的源码、配置模板、迁移、公共入口和已构建静态资源，不包含：

```text
.env
node_modules/
vendor/
runtime/
tests/
.phpunit.cache/
```

上传到宝塔后解压，再继续执行下面的 Composer 安装和 `.env` 配置步骤。

### 4. 安装 PHP 依赖

进入项目目录：

```bash
cd /www/wwwroot/vanillapay/website
composer install --no-dev --optimize-autoloader
```

如果宝塔终端里的 `composer` 命令不可用，可以在宝塔软件商店安装 Composer，或使用 PHP 执行 `composer.phar`。

### 5. 配置环境变量

复制示例配置：

```bash
cp .example.env .env
```

编辑 `.env`：

```ini
APP_DEBUG = false
APP_KEY = 换成至少32位的随机字符串

DB_DRIVER = mysql
DB_TYPE = mysql
DB_HOST = 127.0.0.1
DB_NAME = vanillapay
DB_USER = vanillapay
DB_PASS = 你的数据库强密码
DB_PORT = 3306
DB_CHARSET = utf8mb4
DB_PREFIX = vp_

DEFAULT_LANG = zh-cn
```

`APP_DEBUG` 生产必须为 `false`。`APP_KEY` 会用于密码重置令牌签名，不能使用示例值。

### 6. 设置目录权限

宝塔默认运行用户通常是 `www`：

```bash
cd /www/wwwroot/vanillapay/website
chown -R www:www runtime public/static
chmod -R 755 runtime public/static
```

如后续阶段启用上传目录，再给对应上传目录写权限。

### 7. 创建宝塔网站

宝塔面板进入 `网站` -> `添加站点`：

```text
域名：你的域名
根目录：/www/wwwroot/vanillapay/website
PHP版本：选择 PHP 8.4 或 PHP 8.5
数据库：不在这里创建也可以，已在第2步创建
```

创建后进入站点设置：

1. `网站目录` -> `运行目录` 选择 `/public`。
2. `默认文档` 确认包含 `index.php`。
3. `SSL` 配置证书并开启 HTTPS。

如果无法设置运行目录为 `/public`，不要把整个项目目录暴露为 Web 根目录。可以把站点根目录直接设为：

```text
/www/wwwroot/vanillapay/website/public
```

此时需要确认宝塔的 `open_basedir` 允许访问上级项目目录，否则 ThinkPHP 无法读取 `app/`、`config/`、`vendor/`。

### 8. 配置伪静态

Nginx 站点设置中打开 `伪静态`，写入：

```nginx
location / {
    if (!-e $request_filename) {
        rewrite ^(.*)$ /index.php?s=/$1 last;
        break;
    }
}
```

如果你没有把运行目录设置为 `/public`，额外加上敏感目录保护：

```nginx
location ~ ^/(\.env|app|config|database|extend|runtime|tests|vendor|composer\.(json|lock)|package(-lock)?\.json|phpunit\.xml|README\.md) {
    deny all;
}
```

正常情况下，运行目录为 `/public` 后，上述敏感目录不会暴露到 Web。

### 9. 执行数据库迁移

确认 `.env` 数据库连接无误后执行：

```bash
cd /www/wwwroot/vanillapay/website
php think migrate:run
```

如果宝塔服务器安装了多个 PHP 版本，建议使用站点对应的 PHP 完整路径执行，例如：

```bash
/www/server/php/85/bin/php think migrate:run
```

本阶段会创建：

```text
vp_users
vp_admins
```

检查迁移状态：

```bash
php think migrate:status
```

### 10. Tailwind CSS 构建

仓库中应部署已构建的：

```text
public/static/dist/app.css
```

如果你修改了 `view/` 模板或 Tailwind 配置，需要重新构建 CSS：

```bash
cd /www/wwwroot/vanillapay/website
npm install
npm run build:css
```

生产服务器不改前端样式时，不需要每次部署都执行 `npm install`。

### 11. 验证站点

命令行检查：

```bash
php think route:list
curl -I https://你的域名/login
curl -I https://你的域名/dashboard
curl -I https://你的域名/static/dist/app.css
```

浏览器检查：

1. 访问 `https://你的域名/register` 注册商户。
2. 注册成功后跳转登录页。
3. 使用新账号登录后进入 `/dashboard` 商户首页。
4. 退出后访问 `/dashboard` 会跳转到 `/login`。
5. 错误密码连续 5 次后账号会临时锁定。

## 本地开发命令

```bash
composer install
npm install
npm run build:css
php think run -p 8080
```

访问：

```text
http://127.0.0.1:8080/login
```

运行测试：

```bash
vendor/bin/phpunit
```

Windows 环境可使用：

```powershell
vendor\bin\phpunit
```

## 常见问题

### 访问页面 404

优先检查：

- 宝塔站点运行目录是否为 `/public`。
- 宝塔 `默认文档` 是否包含 `index.php`，否则访问域名根路径 `/` 可能被 Nginx 当成目录返回 404。
- Nginx 伪静态是否已配置。
- `php think route:list` 是否能看到 `/login`、`/register`、`dashboard`。

### 页面提示 Driver [Think] not supported

说明模板驱动未安装或 Composer 依赖不完整，执行：

```bash
composer install --no-dev --optimize-autoloader
```

确认 `composer.json` 中存在：

```json
"topthink/think-view": "^2.0"
```

### 数据库表不存在

确认 `.env` 数据库配置正确后执行：

```bash
php think migrate:run
```

项目使用 `DB_PREFIX = vp_`，所以实际表名是 `vp_users`、`vp_admins`。

如果数据库中出现了 `vp_vp_users`、`vp_vp_admins`，说明使用过旧版迁移文件重复套用了表前缀。新站且表内没有有效数据时，可以清理后重新迁移：

```bash
mysql -u vanillapay -p vanillapay -e "DROP TABLE IF EXISTS vp_vp_users, vp_vp_admins; DELETE FROM vp_migrations WHERE version IN ('20260603065432','20260603065433');"
/www/server/php/85/bin/php think migrate:run
```

如果 `vp_vp_users` 中已经有需要保留的数据，不要删除，改为重命名：

```bash
mysql -u vanillapay -p vanillapay -e "RENAME TABLE vp_vp_users TO vp_users, vp_vp_admins TO vp_admins;"
```

### 样式不生效

确认文件存在并可访问：

```text
public/static/dist/app.css
```

浏览器访问：

```text
https://你的域名/static/dist/app.css
```

如果返回 404，检查宝塔运行目录和文件权限。

## 后续阶段提醒

P2/P3/P4/P5 完成后会新增设备端口、网关回调、商户中心、后台和定时任务。届时需要在宝塔 `计划任务` 中增加对应命令，例如：

```bash
php /www/wwwroot/vanillapay/website/think vanilla:order-expire
php /www/wwwroot/vanillapay/website/think vanilla:device-check
php /www/wwwroot/vanillapay/website/think vanilla:callback-retry
```

当前 P1 阶段暂不需要计划任务。
