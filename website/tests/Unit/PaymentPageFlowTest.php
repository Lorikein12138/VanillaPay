<?php

use PHPUnit\Framework\TestCase;

final class PaymentPageFlowTest extends TestCase
{
    public function testPaymentPageShowsProductAndUsesChannelThemes(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/gateway/pay.html') ?: '';
        $controller = file_get_contents($root . '/app/gateway/controller/PayPage.php') ?: '';

        foreach (['商品名', '{$order.product_name}', '付款渠道', '{$channelName}', 'channelTheme', 'wxpay', 'emerald', 'alipay', 'sky'] as $text) {
            $this->assertStringContainsString($text, $template . $controller);
        }

        $this->assertStringContainsString('data-success="/pay/success/{$order.order_no}"', $template);
        $this->assertStringContainsString('data-return="{$order.return_url}"', $template);
        $this->assertStringContainsString('/static/vendor/poll.js?v={:asset_version(\'/static/vendor/poll.js\')}', $template);
        $this->assertStringNotContainsString('rounded-full bg-white/15', $template);
        $this->assertStringNotContainsString('rounded-md bg-emerald-50 px-2 py-1', $template);
        $this->assertStringNotContainsString('rounded-md bg-sky-50 px-2 py-1', $template);
    }

    public function testPaymentPollShowsSuccessMessageBeforeSuccessPageRedirect(): void
    {
        $poll = file_get_contents(dirname(__DIR__, 2) . '/public/static/vendor/poll.js') ?: '';

        $this->assertStringContainsString('successUrl', $poll);
        $this->assertStringContainsString('showSuccess', $poll);
        $this->assertStringContainsString('支付成功', $poll);
        $this->assertStringContainsString('setTimeout', $poll);
        $this->assertStringContainsString('window.location.href = targetUrl', $poll);
        $this->assertStringNotContainsString('window.location.href = returnUrl', $poll);
    }

    public function testPaymentSuccessRouteControllerAndPage(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/gateway.php') ?: '';
        $controller = file_get_contents($root . '/app/gateway/controller/PayPage.php') ?: '';
        $templatePath = $root . '/view/gateway/success.html';

        $this->assertStringContainsString("Route::get('pay/success/<order_no>'", $route);
        $this->assertStringContainsString('public function success(string $order_no)', $controller);
        $this->assertFileExists($templatePath);

        $template = file_get_contents($templatePath) ?: '';
        foreach ([
            '支付成功',
            '商品名',
            '{$order.product_name}',
            '付款渠道',
            '{$channelName}',
            '付款金额',
            '{$order.real_amount}',
            '付款时间',
            '{$paidAt}',
            'data-return-url="{$returnUrl}"',
            'data-countdown="5"',
            '5 秒后自动跳转',
        ] as $text) {
            $this->assertStringContainsString($text, $template);
        }
    }

    public function testPaymentPageFitsDesktopViewportWithCompactTwoColumnLayout(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/gateway/pay.html') ?: '';

        foreach ([
            'lg:flex lg:items-center',
            'lg:max-w-5xl',
            'lg:h-[calc(100vh-3rem)]',
            'lg:max-h-[680px]',
            'lg:grid-cols-[minmax(0,0.92fr)_minmax(0,1.08fr)]',
            'lg:h-[min(32vh,220px)]',
            'lg:py-1.5',
            'lg:self-center',
            'lg:justify-between',
            'lg:flex-1 lg:items-center lg:justify-center',
            '订单详情',
            'inline-flex w-fit',
            'p-1.5',
        ] as $text) {
            $this->assertStringContainsString($text, $template);
        }

        $this->assertStringNotContainsString('mx-auto max-w-md', $template);
        $this->assertStringNotContainsString('bg-white/95 p-3', $template);
    }

    public function testPaymentPageRedirectsAndRemovesQrWhenOrderExpires(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/gateway.php') ?: '';
        $controller = file_get_contents($root . '/app/gateway/controller/PayPage.php') ?: '';
        $template = file_get_contents($root . '/view/gateway/pay.html') ?: '';
        $poll = file_get_contents($root . '/public/static/vendor/poll.js') ?: '';
        $expiredTemplatePath = $root . '/view/gateway/expired.html';

        $this->assertStringContainsString("Route::get('pay/expired/<order_no>'", $route);
        $this->assertStringContainsString('public function expired(string $order_no)', $controller);
        $this->assertStringContainsString('OrderExpirationService', $controller);
        $this->assertStringContainsString('$this->expiration->refresh();', $controller);
        $this->assertStringContainsString("'expired' =>", $controller);
        $this->assertStringContainsString("'expired_url' =>", $controller);
        $this->assertStringContainsString('data-expired="/pay/expired/{$order.order_no}"', $template);
        $this->assertStringContainsString('id="qr-panel"', $template);
        $this->assertStringContainsString('expiredUrl', $poll);
        $this->assertStringContainsString('data.expired', $poll);
        $this->assertStringContainsString('window.location.replace(targetUrl)', $poll);
        $this->assertFileExists($expiredTemplatePath);

        $expiredTemplate = file_get_contents($expiredTemplatePath) ?: '';
        foreach (['订单已超时', '付款二维码已失效', '{$order.order_no}', '{$order.out_trade_no}', '{$returnUrl}'] as $text) {
            $this->assertStringContainsString($text, $expiredTemplate);
        }
    }
}
