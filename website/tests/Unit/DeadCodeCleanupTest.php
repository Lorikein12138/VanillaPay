<?php

use PHPUnit\Framework\TestCase;

final class DeadCodeCleanupTest extends TestCase
{
    public function testLegacyLoggingOrderPaidHandlerWasRemoved(): void
    {
        $this->assertFileDoesNotExist(dirname(__DIR__, 2) . '/app/common/contract/LoggingOrderPaidHandler.php');
    }

    public function testDefaultThinkphpWelcomeControllerWasRemoved(): void
    {
        $root = dirname(__DIR__, 2);

        $this->assertFileDoesNotExist($root . '/app/controller/Index.php');
        foreach (['route/index.php', 'route/gateway.php', 'route/app.php', 'route/console.php'] as $routeFile) {
            $this->assertStringNotContainsString('\app\controller\Index', file_get_contents($root . '/' . $routeFile) ?: '');
        }
    }
}
