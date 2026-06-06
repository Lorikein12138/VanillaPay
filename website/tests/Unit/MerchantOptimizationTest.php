<?php

use PHPUnit\Framework\TestCase;

final class MerchantOptimizationTest extends TestCase
{
    public function testDashboardShowsFloatStepInsteadOfFloatMax(): void
    {
        $dashboard = file_get_contents(dirname(__DIR__, 2) . '/view/index/dashboard.html') ?: '';

        $this->assertStringNotContainsString('{$user.float_mode} / {$user.float_step}', $dashboard);
        $this->assertStringNotContainsString('{$user.float_mode} / {$user.float_max}', $dashboard);
    }

    public function testDashboardIsRenamedAndShowsOrderAndAmountSummaryCards(): void
    {
        $root = dirname(__DIR__, 2);
        $dashboard = file_get_contents($root . '/view/index/dashboard.html') ?: '';
        $layout = file_get_contents($root . '/view/index/merchant_layout.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Dashboard.php') ?: '';

        $this->assertStringContainsString('数据面板', $dashboard . $layout);
        $this->assertStringContainsString('商户凭证', $layout);
        $this->assertStringContainsString('收款信息', $layout);
        $this->assertStringContainsString('订单信息', $layout);
        $this->assertStringNotContainsString('>看板<', $layout);
        $this->assertStringNotContainsString('数据版', $dashboard . $layout);
        $this->assertStringNotContainsString('API 凭证', $layout);
        foreach (['订单数', '总订单数', '已支付', '待支付', '过期订单数', '金额流水', '已支付订单金额数', '已支付支付宝订单', '已支付微信订单'] as $text) {
            $this->assertStringContainsString($text, $dashboard);
        }
        $this->assertStringNotContainsString('创建测试订单', $dashboard);
        $this->assertStringNotContainsString('href="/order-test"', $dashboard);

        foreach (['{$user.username}', '{$user.pid}', '商户号', '浮动规则', '{$user.float_mode}', '{$user.float_step}'] as $oldText) {
            $this->assertStringNotContainsString($oldText, $dashboard);
        }

        foreach (['totalOrders', 'paidOrders', 'pendingOrders', 'expiredOrders', 'paidAmount', 'paidAlipayAmount', 'paidWxpayAmount', 'sumByUser'] as $text) {
            $this->assertStringContainsString($text, $controller . $dashboard);
        }
    }

    public function testQrcodePageUsesFixedChannelNamesAndSingleCodePerChannel(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/qrcodes.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Qrcodes.php') ?: '';
        $repository = file_get_contents($root . '/app/common/repository/QrcodeRepositoryInterface.php') ?: '';
        $thinkRepository = file_get_contents($root . '/app/common/repository/ThinkQrcodeRepository.php') ?: '';

        $this->assertStringNotContainsString('name="name"', $template);
        $this->assertStringContainsString('收款信息', $template);
        $this->assertStringContainsString('wxpayQr', $template . $controller);
        $this->assertStringContainsString('alipayQr', $template . $controller);
        $this->assertStringContainsString('value="wxpay"', $template);
        $this->assertStringContainsString('value="alipay"', $template);
        $this->assertStringContainsString('微信收款', $template);
        $this->assertStringContainsString('支付宝收款', $template);
        $this->assertStringContainsString("'name' => \$channel", $controller);
        $this->assertStringContainsString('QrcodeImageProcessor', $controller);
        $this->assertStringContainsString('deleteForUserChannel', $controller);
        $this->assertStringContainsString('deleteForUserChannel', $repository);
        $this->assertStringContainsString("whereIn('channel', ['wxpay', 'alipay'])", $thinkRepository);
        $this->assertStringContainsString('isset($latestByChannel[$channel])', $thinkRepository);
    }

    public function testQrcodePageDoesNotShowRemovedUploadHint(): void
    {
        $template = file_get_contents(dirname(__DIR__, 2) . '/view/index/qrcodes.html') ?: '';

        $this->assertStringNotContainsString('每个商户最多保留 2 张收款码', $template);
        $this->assertStringNotContainsString('图片会自动裁剪外部边框并清理中心头像区域', $template);
        $this->assertStringNotContainsString('当前限制：单文件', $template);
        $this->assertStringNotContainsString('upload_max_filesize', $template);
        $this->assertStringNotContainsString('post_max_size', $template);
    }

    public function testQrcodeUploadShowsRuntimeDiagnosticsAndVisibleErrors(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/qrcodes.html') ?: '';
        $layout = file_get_contents($root . '/view/index/merchant_layout.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Qrcodes.php') ?: '';

        foreach (['uploadDiagnostics', 'GD 扩展', '上传目录可写'] as $text) {
            $this->assertStringContainsString($text, $template . $controller);
        }
        $this->assertStringContainsString("Session::flash('flash_tone', 'error')", $controller);
        $this->assertStringContainsString('Log::error', $controller);
        $this->assertStringContainsString('flash_tone', $layout);
        $this->assertStringContainsString('border-rose-200', $layout);
    }

    public function testQrcodeUploadCapturesMimeBeforeMovingUpload(): void
    {
        $controller = file_get_contents(dirname(__DIR__, 2) . '/app/index/controller/Qrcodes.php') ?: '';

        $this->assertStringContainsString('$mime = (string) $file->getMime();', $controller);
        $this->assertStringContainsString('$this->validator->validate($mime, (int) $file->getSize());', $controller);
        $this->assertStringContainsString('$name = date(\'YmdHis\') . \'_\' . bin2hex(random_bytes(8)) . \'.png\';', $controller);
        $this->assertStringContainsString('$qrContent = $this->processor->process($movedPath, $mime);', $controller);
        $this->assertStringContainsString("'qr_content' => \$qrContent", $controller);
        $this->assertStringNotContainsString('$this->processor->process($movedPath, (string) $file->getMime());', $controller);
    }

    public function testDevicePageAndControllerAllowOnlyOneDevicePerMerchant(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/devices.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Devices.php') ?: '';

        $this->assertStringContainsString('currentDevice', $controller);
        $this->assertStringContainsString('ensureDevice', $controller);
        $this->assertStringContainsString('$currentDevice', $template);
        $this->assertStringNotContainsString('单商户单设备', $template);
        $this->assertStringNotContainsString('设备名称', $template);
        $this->assertStringNotContainsString('name="name"', $template);
        $this->assertStringNotContainsString('重新绑定前请先删除当前设备', $template);
        $this->assertStringNotContainsString('<table', $template);
    }

    public function testDevicePageShowsScanBindingQrCode(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/devices.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Devices.php') ?: '';

        $this->assertStringContainsString('binding_qr', $controller);
        $this->assertStringContainsString('QRCode', $controller);
        $this->assertStringContainsString('绑定二维码', $template);
        $this->assertStringContainsString('绑定串', $template);
        $this->assertStringContainsString('{$currentDevice.binding_qr}', $template);
        foreach (['设备状态', '最新心跳时间', '版本'] as $text) {
            $this->assertStringContainsString($text, $template);
        }
    }

    public function testCallbackTestIsReplacedByOrderTest(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/index.php') ?: '';
        $layout = file_get_contents($root . '/view/index/merchant_layout.html') ?: '';
        $controllerPath = $root . '/app/index/controller/OrderTest.php';
        $templatePath = $root . '/view/index/order_test.html';

        $this->assertFileExists($controllerPath);
        $this->assertFileExists($templatePath);
        $this->assertStringContainsString("Route::get('order-test'", $route);
        $this->assertStringContainsString("Route::post('order-test'", $route);
        $this->assertStringNotContainsString('callback-test', $route . $layout);
        $this->assertStringContainsString('/order-test', $layout);
        $this->assertStringContainsString('订单测试', $layout);

        $controller = file_get_contents($controllerPath) ?: '';
        $template = file_get_contents($templatePath) ?: '';
        $this->assertStringContainsString('OrderCreationService', $controller);
        $this->assertStringContainsString('CreateOrderInput', $controller);
        $this->assertStringContainsString("redirect('/pay/' . \$order['order_no'])", $controller);
        $this->assertStringContainsString('name="money"', $template);
        $this->assertStringContainsString('name="channel"', $template);
        $this->assertStringContainsString('value="wxpay"', $template);
        $this->assertStringContainsString('value="alipay"', $template);
    }

    public function testDocsAreCategorizedAndCoverRequiredOperations(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/docs.html') ?: '';
        $epay = file_get_contents($root . '/app/gateway/controller/Epay.php') ?: '';

        foreach (['易支付', '码支付', '源支付', '创建订单', '查询订单信息', '查询订单状态', '关闭订单', '查询服务端状态', '回调参数说明'] as $text) {
            $this->assertStringContainsString($text, $template);
        }

        foreach (['submit.php', 'mapi.php', 'creat_order/', 'yuanpay/submit', 'yuanpay/mapi'] as $endpoint) {
            $this->assertStringContainsString($endpoint, $template);
        }

        foreach (["'order_status'", "'close'", "'server_status'", 'release('] as $implementationText) {
            $this->assertStringContainsString($implementationText, $epay);
        }
    }

    public function testMerchantShellUsesModernTailwindInteractionStyles(): void
    {
        $layout = file_get_contents(dirname(__DIR__, 2) . '/view/index/merchant_layout.html') ?: '';

        $this->assertStringContainsString('shadow-soft', $layout);
        $this->assertStringContainsString('transition', $layout);
        $this->assertStringContainsString('md:hidden', $layout);
        $this->assertStringContainsString('backdrop-blur', $layout);
    }

    public function testWebsiteUsesConfiguredBrandIcons(): void
    {
        $root = dirname(__DIR__, 2);
        $layout = file_get_contents($root . '/view/index/merchant_layout.html') ?: '';

        $this->assertFileExists($root . '/public/favicon.ico');
        $this->assertFileExists($root . '/public/static/brand/VanillaClub.png');
        $this->assertSame(hash_file('sha256', $root . '/img/favicon.ico'), hash_file('sha256', $root . '/public/favicon.ico'));
        $this->assertSame(hash_file('sha256', $root . '/img/VanillaClub.png'), hash_file('sha256', $root . '/public/static/brand/VanillaClub.png'));
        $this->assertStringContainsString('<link rel="icon" href="/favicon.ico"', $layout);
        $this->assertStringContainsString('/static/brand/VanillaClub.png', $layout);
        $this->assertStringContainsString('alt="VanillaPay"', $layout);
    }

    public function testOrderPageSupportsExpiredStatusAndOneClickCleanup(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/index.php') ?: '';
        $template = file_get_contents($root . '/view/index/orders.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Orders.php') ?: '';
        $repository = file_get_contents($root . '/app/common/repository/OrderRepositoryInterface.php') ?: '';
        $thinkRepository = file_get_contents($root . '/app/common/repository/ThinkOrderRepository.php') ?: '';

        $this->assertStringContainsString('<option value="expired"', $template);
        $this->assertStringContainsString('已过期', $template);
        $this->assertStringContainsString('action="/orders/expire"', $template);
        $this->assertStringContainsString('删除过期订单', $template);
        $this->assertStringContainsString("Route::post('orders/expire'", $route);
        $this->assertStringContainsString('OrderExpirationService', $controller);
        $this->assertStringContainsString('$this->expiration->refresh();', $controller);
        $this->assertStringContainsString('deleteExpiredByUser', $controller);
        $this->assertStringContainsString('deleteExpiredByUser', $repository);
        $this->assertStringContainsString('deleteExpiredByUser', $thinkRepository);
    }

    public function testOrderPageProvidesSupplementActionForUnpaidOrders(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/index.php') ?: '';
        $template = file_get_contents($root . '/view/index/orders.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Orders.php') ?: '';

        $this->assertStringContainsString('订单信息', $template);
        $this->assertStringContainsString("Route::post('orders/supplement'", $route);
        $this->assertStringContainsString('OrderSupplementService', $controller);
        $this->assertStringContainsString('action="/orders/supplement"', $template);
        $this->assertStringContainsString('补单', $template);
        $this->assertStringContainsString('$order[\'can_supplement\']', $controller);
        $this->assertStringContainsString('callback_dispatched', $controller);
        $this->assertStringContainsString('catch (\\Throwable $e)', $controller);
        $this->assertStringContainsString('Log::error', $controller);
        $this->assertStringContainsString('disabled', $template);
    }

    public function testOrderViewsRefreshExpiredOrdersBeforeRendering(): void
    {
        $root = dirname(__DIR__, 2);
        $orders = file_get_contents($root . '/app/index/controller/Orders.php') ?: '';
        $dashboard = file_get_contents($root . '/app/index/controller/Dashboard.php') ?: '';

        foreach (['OrderExpirationService', '$this->expiration->refresh();'] as $text) {
            $this->assertStringContainsString($text, $orders);
            $this->assertStringContainsString($text, $dashboard);
        }
    }

    public function testPaymentPageShowsOrderMetadataAndCountdown(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/gateway/pay.html') ?: '';
        $controller = file_get_contents($root . '/app/gateway/controller/PayPage.php') ?: '';

        foreach (['下单时间', '订单号', '商户单号', '剩余支付时间', 'expireAt', 'remainingSeconds'] as $text) {
            $this->assertStringContainsString($text, $template . $controller);
        }
    }
}
