#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

find_php() {
  if [ -n "${PHP_BIN:-}" ]; then
    printf '%s\n' "$PHP_BIN"
    return
  fi

  for candidate in /www/server/php/85/bin/php /www/server/php/84/bin/php /usr/bin/php php; do
    if command -v "$candidate" >/dev/null 2>&1 || [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      return
    fi
  done

  echo "PHP executable not found. Set PHP_BIN=/path/to/php and rerun." >&2
  exit 1
}

if [ ! -f ".env" ]; then
  echo ".env not found. Run this script from the deployed website root after .env is configured." >&2
  exit 1
fi

PHP_BIN_RESOLVED="$(find_php)"
echo "Using PHP: $PHP_BIN_RESOLVED"

"$PHP_BIN_RESOLVED" <<'PHP'
<?php

function fail(string $message): void
{
    fwrite(STDERR, $message . PHP_EOL);
    exit(1);
}

function load_env_file(string $path): array
{
    $env = [];
    foreach (file($path, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES) ?: [] as $line) {
        $line = trim($line);
        if ($line === '' || $line[0] === '#' || !str_contains($line, '=')) {
            continue;
        }

        [$key, $value] = explode('=', $line, 2);
        $env[trim($key)] = trim(trim($value), "\"'");
    }

    return $env;
}

function table_exists(PDO $pdo, string $table): bool
{
    $statement = $pdo->prepare('SHOW TABLES LIKE ?');
    $statement->execute([$table]);

    return (bool) $statement->fetchColumn();
}

function quote_table(string $table): string
{
    return '`' . str_replace('`', '``', $table) . '`';
}

function ensure_migration_table(PDO $pdo, string $table): void
{
    if (table_exists($pdo, $table)) {
        return;
    }

    $pdo->exec(sprintf(
        'CREATE TABLE %s (
            `version` bigint NOT NULL,
            `migration_name` varchar(100) DEFAULT NULL,
            `start_time` timestamp NULL DEFAULT NULL,
            `end_time` timestamp NULL DEFAULT NULL,
            `breakpoint` tinyint(1) NOT NULL DEFAULT 0,
            PRIMARY KEY (`version`)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci',
        quote_table($table)
    ));
}

$env = load_env_file(__DIR__ . '/.env');
$get = fn (string $key, string $default = ''): string => $env[$key] ?? $default;

if ($get('DB_NAME') === '') {
    fail('DB_NAME is empty in .env.');
}

$prefix = $get('DB_PREFIX', '');
$pdo = new PDO(
    sprintf(
        'mysql:host=%s;port=%s;dbname=%s;charset=%s',
        $get('DB_HOST', '127.0.0.1'),
        $get('DB_PORT', '3306'),
        $get('DB_NAME'),
        $get('DB_CHARSET', 'utf8mb4')
    ),
    $get('DB_USER'),
    $get('DB_PASS'),
    [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION, PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC]
);

$baseMigrations = [
    ['users', '20260603065432', 'CreateUsersTable'],
    ['admins', '20260603065433', 'CreateAdminsTable'],
    ['channels', '20260603100000', 'CreateChannelsTable'],
    ['merchant_qrcodes', '20260603100001', 'CreateMerchantQrcodesTable'],
    ['devices', '20260603100002', 'CreateDevicesTable'],
    ['orders', '20260603100003', 'CreateOrdersTable'],
    ['order_amount_lock', '20260603100004', 'CreateOrderAmountLockTable'],
    ['risk_events', '20260603100005', 'CreateRiskEventsTable'],
    ['callback_logs', '20260603100006', 'CreateCallbackLogsTable'],
    ['settings', '20260603100007', 'CreateSettingsTable'],
    ['login_logs', '20260603100008', 'CreateLoginLogsTable'],
    ['operation_logs', '20260603100009', 'CreateOperationLogsTable'],
];

$migrationTable = $prefix . 'migrations';
$loggedVersions = [];
if (table_exists($pdo, $migrationTable)) {
    foreach ($pdo->query('SELECT version FROM ' . quote_table($migrationTable)) as $row) {
        $loggedVersions[(string) $row['version']] = true;
    }
}

$missingStarted = false;
$toBaseline = [];
$inconsistent = [];

foreach ($baseMigrations as [$logicalTable, $version, $name]) {
    $physicalTable = $prefix . $logicalTable;
    $exists = table_exists($pdo, $physicalTable);
    $logged = isset($loggedVersions[$version]);

    printf("%s %-32s table=%s migration=%s\n", $exists ? 'FOUND ' : 'MISS  ', $physicalTable, $exists ? 'exists' : 'missing', $logged ? 'logged' : 'down');

    if ($logged && !$exists) {
        $inconsistent[] = "{$physicalTable} is missing but {$version} is already logged";
        continue;
    }

    if (!$exists) {
        $missingStarted = true;
        continue;
    }

    if ($missingStarted) {
        $inconsistent[] = "{$physicalTable} exists after an earlier base table is missing";
        continue;
    }

    if (!$logged) {
        $toBaseline[] = [$version, $name];
    }
}

if ($inconsistent !== []) {
    fwrite(STDERR, PHP_EOL . "Database shape is not a safe contiguous baseline:" . PHP_EOL);
    foreach ($inconsistent as $message) {
        fwrite(STDERR, " - {$message}" . PHP_EOL);
    }
    fail('Stop here and inspect the database manually before running migrations.');
}

if ($toBaseline === []) {
    echo PHP_EOL . "No baseline records need to be inserted." . PHP_EOL;
    echo "Next: bash deploy-server.sh" . PHP_EOL;
    exit(0);
}

ensure_migration_table($pdo, $migrationTable);

$pdo->beginTransaction();
$statement = $pdo->prepare(
    'INSERT IGNORE INTO ' . quote_table($migrationTable) . ' (version, migration_name, start_time, end_time, breakpoint) VALUES (?, ?, NOW(), NOW(), 0)'
);

foreach ($toBaseline as [$version, $name]) {
    $statement->execute([$version, $name]);
    echo "BASELINED {$version} {$name}" . PHP_EOL;
}

$pdo->commit();

echo PHP_EOL . "Baseline inserted. Next: bash deploy-server.sh" . PHP_EOL;
PHP
