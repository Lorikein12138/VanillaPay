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

    public function testBrandHomePageHasNavigationLoginAndSignedInDashboardLink(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $template);
        $this->assertStringContainsString('href="/login"', $template);
        $this->assertStringContainsString('PID', $template);
        $this->assertStringContainsString('href="/dashboard"', $template);
        $this->assertStringNotContainsString('href="/logout"', $template);
        $this->assertStringNotContainsString('<details', $template);
    }

    public function testSignedInHomeNavigationUsesCompactPidDashboardLinkWithoutDropdownArrow(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringNotContainsString('sm:inline-flex">数据面板</a>', $template);
        $this->assertStringContainsString('<a href="/dashboard" class="inline-flex h-10 max-w-[180px]', $template);
        $this->assertStringContainsString('truncate text-right font-mono', $template);
        $this->assertStringNotContainsString('<summary', $template);
        $this->assertStringNotContainsString('进入数据面板', $template);
        $this->assertStringNotContainsString('退出登录', $template);
        $this->assertStringNotContainsString('⌄', $template);
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

        $this->assertStringContainsString('个人免签支付网关', $template);
        $this->assertStringContainsString('个人商户', $template);
        $this->assertStringContainsString('Android 监听应用', $template);
        $this->assertStringContainsString('开源项目', $template);
        $this->assertStringContainsString('三种协议', $template);

        foreach (['bg-gradient-to-br', 'ring-1', 'backdrop-blur', 'shadow-2xl', 'supports-[backdrop-filter]'] as $class) {
            $this->assertStringContainsString($class, $template);
        }
    }

    public function testBrandHomePageHighlightsPaymentGatewayAdvantages(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        foreach ([
            '易支付',
            '码支付',
            '源支付',
            'D0到账',
            '0手续费',
            'Android 监听应用',
            '免签收款',
            '订单核销',
            '回调通知',
        ] as $text) {
            $this->assertStringContainsString($text, $template);
        }
    }

    public function testBrandHomePageUsesGeneratedTailwindBackgroundClasses(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';
        $css = file_get_contents(dirname(__DIR__, 2) . '/public/static/dist/app.css') ?: '';

        $this->assertStringNotContainsString('bg-white/88', $template);
        $this->assertStringContainsString('bg-white/90', $template);
        $this->assertStringContainsString('.bg-white\\/90', $css);
        $this->assertStringContainsString('.bg-zinc-950', $css);
    }

    public function testBrandHomePageDoesNotDuplicateCoreClaimsAcrossHeroAndVisual(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        foreach ([
            'D0到账',
            '0手续费',
            'Android 监听应用',
            '开源项目',
            '易支付',
            '码支付',
            '源支付',
        ] as $text) {
            $this->assertSame(1, substr_count($template, $text), $text . ' should appear once on the public home page.');
        }

        foreach ([
            'Gateway Console',
            '网关亮点',
            '三协议订单统一核销',
            'submit.php / api.php',
            'creat_order',
            'yuanpay/mapi',
        ] as $text) {
            $this->assertStringNotContainsString($text, $template);
        }
    }

    public function testBrandHomePageFitsDesktopViewportWithoutTechMotion(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/index/home.html') ?: '';

        $this->assertStringContainsString('h-[calc(100vh-4rem)]', $template);
        $this->assertStringContainsString('overflow-hidden', $template);
        $this->assertStringNotContainsString('pb-16', $template);
        $this->assertStringNotContainsString('md:grid-cols-4', $template);
        $this->assertStringContainsString('lg:grid-cols-[minmax(0,1.04fr)_minmax(0,0.96fr)]', $template);
        $this->assertStringContainsString('max-w-[1500px]', $template);
        $this->assertStringContainsString('lg:text-[4.6rem]', $template);

        foreach ([
            '交易链路',
            '订单创建',
            '金额匹配',
            '回调完成',
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
