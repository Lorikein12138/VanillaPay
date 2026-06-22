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

find_composer() {
  if [ -n "${COMPOSER_BIN:-}" ]; then
    printf '%s\n' "$COMPOSER_BIN"
    return
  fi

  for candidate in /usr/bin/composer /usr/local/bin/composer composer; do
    if command -v "$candidate" >/dev/null 2>&1 || [ -f "$candidate" ]; then
      printf '%s\n' "$candidate"
      return
    fi
  done

  echo "Composer not found. Set COMPOSER_BIN=/path/to/composer and rerun." >&2
  exit 1
}

PHP_BIN_RESOLVED="$(find_php)"
COMPOSER_BIN_RESOLVED="$(find_composer)"

if [ ! -f ".env" ]; then
  echo ".env not found. Create it from .example.env before deployment." >&2
  exit 1
fi

mkdir -p runtime public/static/uploads

echo "Using PHP: $PHP_BIN_RESOLVED"
echo "Using Composer: $COMPOSER_BIN_RESOLVED"

"$PHP_BIN_RESOLVED" "$COMPOSER_BIN_RESOLVED" install --no-dev --optimize-autoloader
"$PHP_BIN_RESOLVED" think migrate:run
"$PHP_BIN_RESOLVED" think clear
"$PHP_BIN_RESOLVED" think route:list >/dev/null

if id www >/dev/null 2>&1; then
  chown -R www:www runtime public/static
fi

chmod -R 755 runtime public/static

echo "VanillaPay website deployment update completed."
