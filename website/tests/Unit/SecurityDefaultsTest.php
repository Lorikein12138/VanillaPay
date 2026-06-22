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

    public function testLoginRotatesSessionAndLogoutRequiresCsrfPost(): void
    {
        $root = dirname(__DIR__, 2);
        $userAuth = file_get_contents($root . '/app/index/controller/Auth.php') ?: '';
        $adminAuth = file_get_contents($root . '/app/console/controller/Auth.php') ?: '';
        $indexRoute = file_get_contents($root . '/route/index.php') ?: '';
        $consoleRoute = file_get_contents($root . '/route/console.php') ?: '';
        $consoleAppRoute = file_get_contents($root . '/app/console/route/app.php') ?: '';
        $merchantLayout = file_get_contents($root . '/view/index/merchant_layout.html') ?: '';
        $consoleLayout = file_get_contents($root . '/view/console/layout.html') ?: '';

        $this->assertStringContainsString('session_regenerate_id(true)', $userAuth);
        $this->assertStringContainsString('session_regenerate_id(true)', $adminAuth);
        $this->assertStringContainsString("Route::post('logout'", $indexRoute);
        $this->assertStringContainsString("Route::post('console/logout'", $consoleRoute);
        $this->assertStringContainsString("Route::post('logout'", $consoleAppRoute);
        $this->assertStringNotContainsString("Route::get('logout'", $indexRoute . $consoleRoute . $consoleAppRoute);
        $this->assertStringContainsString('method="post" action="/logout"', $merchantLayout);
        $this->assertStringContainsString('method="post" action="/console/logout"', $consoleLayout);
        $this->assertStringContainsString('name="_csrf"', $merchantLayout . $consoleLayout);
        $this->assertStringNotContainsString('href="/logout"', $merchantLayout);
        $this->assertStringNotContainsString('href="/console/logout"', $consoleLayout);
    }
}
