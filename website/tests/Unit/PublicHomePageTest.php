<?php

use PHPUnit\Framework\TestCase;

final class PublicHomePageTest extends TestCase
{
    public function testRootRouteIsPublicBrandHomePage(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/index.php') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Index.php') ?: '';

        $this->assertStringContainsString("Route::rule('', '\\app\\index\\controller\\Index@home', 'GET|HEAD');", $route);
        $this->assertStringContainsString("Route::rule('/', '\\app\\index\\controller\\Index@home', 'GET|HEAD');", $route);
        $this->assertStringContainsString("Route::rule('home', '\\app\\index\\controller\\Index@home', 'GET|HEAD');", $route);
        $this->assertStringNotContainsString("Route::get('/', '\\app\\index\\controller\\Index@home')->middleware", $route);
        $this->assertStringContainsString("View::fetch('index/home'", $controller);
        $this->assertStringNotContainsString("return redirect('/login');", $controller);
        $this->assertStringNotContainsString("return redirect('/dashboard');", $controller);
    }

    public function testBrandHomePageHasNavigationLoginAndSignedInMenu(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $template);
        $this->assertStringContainsString('href="/login"', $template);
        $this->assertStringContainsString('PID', $template);
        $this->assertStringContainsString('href="/logout"', $template);
        $this->assertStringContainsString('<details', $template);
    }
}
