<?php

use PHPUnit\Framework\TestCase;

final class ConsoleDashboardOptimizationTest extends TestCase
{
    public function testConsoleDashboardUsesRepositoryMetricsInsteadOfDirectQueries(): void
    {
        $controller = file_get_contents(dirname(__DIR__, 2) . '/app/console/controller/Dashboard.php') ?: '';

        $this->assertStringContainsString('dashboardMetricsAll()', $controller);
        $this->assertStringNotContainsString('paginateAll([], 1, 1)', $controller);
        $this->assertStringNotContainsString('use think\\facade\\Db;', $controller);
        $this->assertStringNotContainsString('sumPaidAmount', $controller);
    }
}
