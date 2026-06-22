<?php

use PHPUnit\Framework\TestCase;

final class LoginRedirectRouteTest extends TestCase
{
    public function testLoginRedirectsToDashboardRouteInsteadOfSiteRoot(): void
    {
        $route = file_get_contents(dirname(__DIR__, 2) . '/route/index.php');
        $auth = file_get_contents(dirname(__DIR__, 2) . '/app/index/controller/Auth.php');

        $this->assertStringContainsString("Route::get('dashboard'", $route);
        $this->assertStringContainsString("redirect('/dashboard')", $auth);
        $this->assertStringNotContainsString("return redirect('/');", $auth);
    }
}
