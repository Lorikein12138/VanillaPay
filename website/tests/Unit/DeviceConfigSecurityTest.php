<?php
use PHPUnit\Framework\TestCase;

final class DeviceConfigSecurityTest extends TestCase
{
    public function testConfigRouteIsRateLimitedAndAuthenticated(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/app.php') ?: '';
        $controller = file_get_contents($root . '/app/device/controller/Config.php') ?: '';

        $this->assertStringContainsString("Route::get('app/config'", $route);
        $this->assertStringContainsString("->middleware(\\app\\middleware\\RateLimit::class, 'device', 240, 60)", $route);
        $this->assertStringContainsString('DeviceSigner $signer', $controller);
        $this->assertStringContainsString('$this->signer->verify', $controller);
    }
}
