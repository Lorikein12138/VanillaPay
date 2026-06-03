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
        $this->assertStringContainsString('data-copy-payload', $template);
        $this->assertStringContainsString('绑定串', $template);
    }
}
