<?php

use PHPUnit\Framework\TestCase;

final class MerchantOptimizationTest extends TestCase
{
    public function testDashboardShowsFloatStepInsteadOfFloatMax(): void
    {
        $dashboard = file_get_contents(dirname(__DIR__, 2) . '/view/index/dashboard.html') ?: '';

        $this->assertStringContainsString('{$user.float_mode} / {$user.float_step}', $dashboard);
        $this->assertStringNotContainsString('{$user.float_mode} / {$user.float_max}', $dashboard);
    }

    public function testQrcodePageUsesFixedChannelNamesAndSingleCodePerChannel(): void
    {
        $root = dirname(__DIR__, 2);
        $template = file_get_contents($root . '/view/index/qrcodes.html') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Qrcodes.php') ?: '';
        $repository = file_get_contents($root . '/app/common/repository/QrcodeRepositoryInterface.php') ?: '';
        $thinkRepository = file_get_contents($root . '/app/common/repository/ThinkQrcodeRepository.php') ?: '';

        $this->assertStringNotContainsString('name="name"', $template);
        $this->assertStringContainsString("'name' => \$channel", $controller);
        $this->assertStringContainsString('QrcodeImageProcessor', $controller);
        $this->assertStringContainsString('deleteForUserChannel', $controller);
        $this->assertStringContainsString('deleteForUserChannel', $repository);
        $this->assertStringContainsString("whereIn('channel', ['wxpay', 'alipay'])", $thinkRepository);
        $this->assertStringContainsString('isset($latestByChannel[$channel])', $thinkRepository);
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
        $this->assertStringContainsString('已有设备', $controller);
        $this->assertStringContainsString('$currentDevice', $template);
        $this->assertStringContainsString('重新绑定前请先删除当前设备', $template);
        $this->assertStringNotContainsString('<table', $template);
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
