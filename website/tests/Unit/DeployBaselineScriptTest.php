<?php

use PHPUnit\Framework\TestCase;

final class DeployBaselineScriptTest extends TestCase
{
    public function testDeploymentPackageIncludesExplicitBaselineScriptForExistingDatabases(): void
    {
        $root = dirname(__DIR__, 2);
        $scriptPath = $root . '/deploy-baseline-existing-db.sh';

        $this->assertFileExists($scriptPath);

        $script = file_get_contents($scriptPath) ?: '';
        $packageScript = file_get_contents($root . '/pack-deploy.bat') ?: '';
        $readme = file_get_contents($root . '/README.md') ?: '';

        $this->assertStringContainsString('deploy-baseline-existing-db.sh', $packageScript);
        $this->assertStringContainsString('deploy-baseline-existing-db.sh', $readme);

        foreach ([
            '20260603065432' => 'CreateUsersTable',
            '20260603065433' => 'CreateAdminsTable',
            '20260603100000' => 'CreateChannelsTable',
            '20260603100001' => 'CreateMerchantQrcodesTable',
            '20260603100002' => 'CreateDevicesTable',
            '20260603100003' => 'CreateOrdersTable',
            '20260603100004' => 'CreateOrderAmountLockTable',
            '20260603100005' => 'CreateRiskEventsTable',
            '20260603100006' => 'CreateCallbackLogsTable',
            '20260603100007' => 'CreateSettingsTable',
            '20260603100008' => 'CreateLoginLogsTable',
            '20260603100009' => 'CreateOperationLogsTable',
        ] as $version => $migrationName) {
            $this->assertStringContainsString($version, $script);
            $this->assertStringContainsString($migrationName, $script);
        }

        $this->assertStringNotContainsString('20260603193000', $script);
        $this->assertStringNotContainsString('AddAuthColumnsToUsersTable', $script);
    }

    public function testDeploymentPackageUsesLinuxCompatibleZipEntries(): void
    {
        $root = dirname(__DIR__, 2);
        $packageScript = file_get_contents($root . '/pack-deploy.bat') ?: '';

        $this->assertStringContainsString('CreateEntryFromFile', $packageScript);
        $this->assertStringContainsString('System.IO.Compression;', $packageScript);
        $this->assertStringContainsString('System.IO.Compression.FileSystem;', $packageScript);
        $this->assertStringContainsString('-replace', $packageScript);
        $this->assertStringNotContainsString('Compress-Archive', $packageScript);
    }
}
