<?php

use PHPUnit\Framework\TestCase;

final class DeviceBindingDisplayTest extends TestCase
{
    public function testDeviceListRendersPersistentBindingPayload(): void
    {
        $root = dirname(__DIR__, 2);
        $controller = file_get_contents($root . '/app/index/controller/Devices.php') ?: '';
        $template = file_get_contents($root . '/view/index/devices.html') ?: '';

        $this->assertStringContainsString('binding_payload', $controller);
        $this->assertStringContainsString("rtrim(\$serverUrl, '/') . '|' . \$currentDevice['id'] . '|' . \$currentDevice['device_key']", $controller);
        $this->assertStringContainsString('{$currentDevice.binding_payload}', $template);
        $this->assertStringNotContainsString('data-copy-payload', $template);
        $this->assertStringContainsString('绑定串', $template);
    }

    public function testDeviceBindingPageAutoRefreshesAfterHeartbeatBinding(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/index.php') ?: '';
        $controller = file_get_contents($root . '/app/index/controller/Devices.php') ?: '';
        $template = file_get_contents($root . '/view/index/devices.html') ?: '';

        $this->assertStringContainsString("Route::get('devices/status'", $route);
        $this->assertStringContainsString('public function status()', $controller);
        $this->assertStringContainsString("'is_bound' => !empty(\$currentDevice['last_heartbeat'] ?? null)", $controller);
        $this->assertStringContainsString("fetch('/devices/status'", $template);
        $this->assertStringContainsString('window.location.reload()', $template);
        $this->assertStringContainsString('setInterval(pollDeviceBinding', $template);
        $this->assertStringContainsString('等待 App 绑定，完成后页面会自动刷新', $template);
    }
}
