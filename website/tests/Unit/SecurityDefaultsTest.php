<?php

use PHPUnit\Framework\TestCase;

final class SecurityDefaultsTest extends TestCase
{
    public function testSqlListenerDefaultsToDisabledWhenAppDebugIsMissing(): void
    {
        $database = file_get_contents(dirname(__DIR__, 2) . '/config/database.php') ?: '';

        $this->assertStringContainsString("'trigger_sql'     => env('APP_DEBUG', false)", $database);
    }

    public function testCookieDefaultsProtectSessionCookies(): void
    {
        $cookie = file_get_contents(dirname(__DIR__, 2) . '/config/cookie.php') ?: '';

        $this->assertStringContainsString("'secure'    => env('COOKIE_SECURE', env('APP_ENV', 'local') === 'production')", $cookie);
        $this->assertStringContainsString("'httponly'  => true", $cookie);
        $this->assertStringContainsString("'samesite'  => 'lax'", $cookie);
    }
}
