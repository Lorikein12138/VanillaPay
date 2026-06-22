<?php

use PHPUnit\Framework\TestCase;

class MigrationPrefixTest extends TestCase
{
    public function testMigrationsDoNotApplyDatabasePrefixManually(): void
    {
        $files = glob(dirname(__DIR__, 2) . '/database/migrations/*.php') ?: [];

        $this->assertNotEmpty($files);

        foreach ($files as $file) {
            $content = file_get_contents($file);

            $this->assertStringNotContainsString("env('DB_PREFIX'", $content, basename($file));
            $this->assertStringNotContainsString('env("DB_PREFIX"', $content, basename($file));
            $this->assertStringNotContainsString('tableName(', $content, basename($file));
        }
    }
}
