<?php

use PHPUnit\Framework\TestCase;

final class UserAuthColumnsMigrationTest extends TestCase
{
    public function testUserRuntimeColumnsHaveAdditiveMigrationAfterInitialUsersMigration(): void
    {
        $migrationDir = dirname(__DIR__, 2) . '/database/migrations';
        $files = glob($migrationDir . '/*.php') ?: [];
        $additiveMigrations = array_filter(
            $files,
            fn (string $file): bool => basename($file) > '20260603065432_create_users_table.php'
        );
        $content = implode("\n", array_map(fn (string $file): string => file_get_contents($file) ?: '', $additiveMigrations));

        foreach (['float_mode', 'float_step', 'float_max', 'order_timeout', 'login_fail_count', 'locked_until', 'last_login_at', 'last_login_ip'] as $column) {
            $this->assertStringContainsString($column, $content, $column . ' must be added by a post-create-users migration');
        }
    }
}
