<?php

use PHPUnit\Framework\TestCase;

final class ConsoleEntryRouteTest extends TestCase
{
    public function testConsoleRootRedirectsToLoginOrDashboardInsteadOfPlaceholder(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/console.php') ?: '';
        $appRoute = file_get_contents($root . '/app/console/route/app.php') ?: '';
        $controller = file_get_contents($root . '/app/console/controller/Index.php') ?: '';

        $this->assertStringContainsString("Route::get('console', '\\app\\console\\controller\\Index@index');", $route);
        $this->assertStringContainsString("Route::get('', '\\app\\console\\controller\\Index@index');", $appRoute);
        $this->assertStringContainsString("redirect('/console/login')", $controller);
        $this->assertStringContainsString("redirect('/console/dashboard')", $controller);
        $this->assertStringContainsString("Session::has('admin_id')", $controller);
        $this->assertStringNotContainsString('console ready', $controller);
    }
}
