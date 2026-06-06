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
}
