<?php

use PHPUnit\Framework\TestCase;

final class ConsoleManagementViewTest extends TestCase
{
    public function testConsoleNavigationUsesBusinessNamesAndRemovesRiskAndReconcile(): void
    {
        $layout = $this->view('layout');
        $route = file_get_contents(dirname(__DIR__, 2) . '/route/console.php') ?: '';
        $appRoute = file_get_contents(dirname(__DIR__, 2) . '/app/console/route/app.php') ?: '';
        $routeConfig = file_get_contents(dirname(__DIR__, 2) . '/config/route.php') ?: '';

        foreach (['数据面板', '商户管理', '订单信息', '设备信息', '支付管理', '系统设置'] as $text) {
            $this->assertStringContainsString($text, $layout);
        }

        foreach (['看板', '>商户<', '>订单<', '>设备<', '渠道/协议', '风控', '风控中心', '对账', '/console/risk', '/console/reconcile'] as $text) {
            $this->assertStringNotContainsString($text, $layout);
        }

        $this->assertStringNotContainsString("Route::get('risk'", $route);
        $this->assertStringNotContainsString("Route::post('risk/handle'", $route);
        $this->assertStringNotContainsString("Route::get('reconcile'", $route);
        $this->assertStringNotContainsString("Route::get('risk'", $appRoute);
        $this->assertStringNotContainsString("Route::get('reconcile'", $appRoute);
        $this->assertStringContainsString("'url_route_must'        => true", $routeConfig);
    }

    public function testConsoleDashboardMirrorsMerchantDashboardMetricCards(): void
    {
        $template = $this->view('dashboard');
        $controller = file_get_contents(dirname(__DIR__, 2) . '/app/console/controller/Dashboard.php') ?: '';

        foreach ([
            '数据面板',
            '订单数',
            '总订单数',
            '已支付',
            '待支付',
            '过期订单数',
            '金额流水',
            '已支付订单金额数',
            '已支付支付宝订单',
            '已支付微信订单',
        ] as $text) {
            $this->assertStringContainsString($text, $template);
        }

        foreach (['看板', '成功订单'] as $text) {
            $this->assertStringNotContainsString($text, $template);
        }

        foreach (['totalOrders', 'paidOrders', 'pendingOrders', 'expiredOrders', 'paidAmount', 'paidAlipayAmount', 'paidWxpayAmount'] as $key) {
            $this->assertStringContainsString('$' . $key, $template);
        }
        $this->assertStringContainsString('dashboardMetricsAll()', $controller);
    }

    public function testConsoleMerchantsTableIsVerticallyAlignedAndRenamed(): void
    {
        $template = $this->view('merchants');

        $this->assertStringContainsString('商户管理', $template);
        $this->assertStringNotContainsString('{block name="title"}商户{/block}', $template);
        $this->assertStringContainsString('align-middle', $template);
        $this->assertStringContainsString('divide-y divide-zinc-100', $template);
        $this->assertStringContainsString('inline-flex', $template);
    }

    public function testConsoleOrdersShowMerchantOrderNoAndUseRenamedTitle(): void
    {
        $template = $this->view('orders');

        $this->assertStringContainsString('{block name="title"}订单信息{/block}', $template);
        $this->assertStringContainsString('订单信息', $template);
        $this->assertStringContainsString('商户单号', $template);
        $this->assertStringContainsString('{$o.out_trade_no}', $template);
        $this->assertStringNotContainsString('全局订单', $template);
    }

    public function testConsoleDevicesDoNotShowDeviceNameColumn(): void
    {
        $template = $this->view('devices');

        $this->assertStringContainsString('{block name="title"}设备信息{/block}', $template);
        $this->assertStringContainsString('设备信息', $template);
        $this->assertStringNotContainsString('<th class="p-3">名称</th>', $template);
        $this->assertStringNotContainsString('{$d.device_name}', $template);
    }

    public function testConsolePaymentAndSettingsPagesAreSimplified(): void
    {
        $channels = $this->view('channels');
        $settings = $this->view('settings');
        $controller = file_get_contents(dirname(__DIR__, 2) . '/app/console/controller/Settings.php') ?: '';
        $dispatcher = file_get_contents(dirname(__DIR__, 2) . '/app/common/contract/CallbackDispatcher.php') ?: '';

        $this->assertStringContainsString('{block name="title"}支付管理{/block}', $channels);
        $this->assertStringContainsString('支付管理', $channels);
        $this->assertStringNotContainsString('渠道/协议', $channels);

        foreach ([
            'SMTP 服务器',
            'SMTP 端口',
            '加密方式',
            'SMTP 用户名',
            'SMTP 密码',
            '发件邮箱',
            '发件名称',
            'smtp_host',
            'smtp_port',
            'smtp_secure',
            'smtp_username',
            'smtp_password',
            'smtp_from_email',
            'smtp_from_name',
        ] as $text) {
            $this->assertStringContainsString($text, $settings);
        }

        foreach (['register_audit', '注册需人工审核', 'notice', '系统公告', '回调发送方式', '立即发送', '加入队列'] as $text) {
            $this->assertStringNotContainsString($text, $settings);
            $this->assertStringNotContainsString($text, $controller);
        }

        foreach (['smtp_host', 'smtp_port', 'smtp_secure', 'smtp_username', 'smtp_from_email', 'smtp_from_name'] as $key) {
            $this->assertStringContainsString("set('{$key}'", $controller);
        }

        $this->assertStringContainsString("set('callback_driver', 'sync')", $controller);
        $this->assertStringNotContainsString("get('callback_driver'", $dispatcher);
        $this->assertStringNotContainsString('queueForRetry', $dispatcher);
    }

    private function view(string $name): string
    {
        return file_get_contents(dirname(__DIR__, 2) . "/view/console/{$name}.html") ?: '';
    }
}
