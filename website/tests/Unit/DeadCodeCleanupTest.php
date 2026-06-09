<?php

use PHPUnit\Framework\TestCase;

final class DeadCodeCleanupTest extends TestCase
{
    public function testLegacyLoggingOrderPaidHandlerWasRemoved(): void
    {
        $this->assertFileDoesNotExist(dirname(__DIR__, 2) . '/app/common/contract/LoggingOrderPaidHandler.php');
    }
}
