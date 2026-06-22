<?php

use PHPUnit\Framework\TestCase;

final class SupplyChainHygieneTest extends TestCase
{
    public function testApplicationComposerLockIsNotIgnored(): void
    {
        $gitignore = file_get_contents(dirname(__DIR__, 2) . '/.gitignore') ?: '';

        $this->assertStringNotContainsString('composer.lock', $gitignore);
    }
}
