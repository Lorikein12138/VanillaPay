<?php

use PHPUnit\Framework\TestCase;

final class EpayPublicEntrypointTest extends TestCase
{
    public function testEpayPhpCompatibilityEntrypointsExistForNginxPhpLocations(): void
    {
        $root = dirname(__DIR__, 2);
        $route = file_get_contents($root . '/route/gateway.php') ?: '';

        foreach (['submit.php', 'mapi.php', 'api.php'] as $entrypoint) {
            $path = $root . '/public/' . $entrypoint;

            $this->assertFileExists($path);
            $this->assertStringContainsString("Route::any('{$entrypoint}'", $route);
            $this->assertStringContainsString("require __DIR__ . '/index.php';", file_get_contents($path) ?: '');
        }
    }
}
