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

    public function testBrandHomePageNavigationUsesFullViewportWidth(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('flex h-16 w-full items-center justify-between', $template);
        $this->assertStringNotContainsString('flex h-16 max-w-7xl items-center justify-between', $template);
    }

    public function testBrandHomePageKeepsAuthActionsOnlyInNavigation(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('href="/login"', $template);
        $this->assertStringNotContainsString('登录商户中心', $template);
        $this->assertStringNotContainsString('href="/register"', $template);
        $this->assertStringNotContainsString('注册商户', $template);
    }

    public function testBrandHomePageIsCommercialShowcaseWithoutConcreteMetrics(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        foreach (['¥ 128.03', '¥ 56.01', '2/2', '2 次', '秒级', 'LIVE'] as $text) {
            $this->assertStringNotContainsString($text, $template);
        }

        $this->assertStringContainsString('产品工作台', $template);
        $this->assertStringContainsString('商户视图预览', $template);
        $this->assertStringContainsString('Android 监听端', $template);
        $this->assertStringContainsString('接入流程', $template);

        foreach (['bg-gradient-to-br', 'ring-1', 'backdrop-blur', 'shadow-2xl', 'supports-[backdrop-filter]'] as $class) {
            $this->assertStringContainsString($class, $template);
        }
    }

    public function testBrandHomePageFitsDesktopViewportWithoutTechMotion(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('h-[calc(100vh-4rem)]', $template);
        $this->assertStringContainsString('overflow-hidden', $template);
        $this->assertStringNotContainsString('pb-16', $template);
        $this->assertStringNotContainsString('md:grid-cols-4', $template);
        $this->assertStringContainsString('lg:grid-cols-[minmax(0,1fr)_minmax(0,0.92fr)]', $template);
        $this->assertStringContainsString('max-w-[1500px]', $template);
        $this->assertStringContainsString('lg:max-w-[640px]', $template);
        $this->assertStringContainsString('lg:text-7xl', $template);

        foreach ([
            '交易链路',
            '通知监听',
            '回调治理',
            '运行监控',
        ] as $text) {
            $this->assertStringContainsString($text, $template);
        }

        foreach ([
            '科技动效',
            '@keyframes',
            'motion-safe-run',
            'animate-[',
            'bg-[radial-gradient',
            'backdrop-blur-2xl',
        ] as $text) {
            $this->assertStringNotContainsString($text, $template);
        }
    }

    public function testMobileHomePageDoesNotLockViewportHeight(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('min-h-screen overflow-x-hidden', $template);
        $this->assertStringContainsString('lg:h-screen lg:overflow-hidden', $template);
        $this->assertStringContainsString('lg:h-[calc(100vh-4rem)]', $template);
        $this->assertStringNotContainsString('relative h-screen overflow-hidden bg', $template);
        $this->assertStringNotContainsString('<main class="h-[calc(100vh-4rem)] overflow-hidden"', $template);
    }
}
