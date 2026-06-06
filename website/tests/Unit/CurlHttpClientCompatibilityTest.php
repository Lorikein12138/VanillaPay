<?php

use PHPUnit\Framework\TestCase;

final class CurlHttpClientCompatibilityTest extends TestCase
{
    public function testDoesNotCallDeprecatedCurlCloseOnPhp85(): void
    {
        $client = file_get_contents(dirname(__DIR__, 2) . '/app/common/support/CurlHttpClient.php') ?: '';

        $this->assertStringNotContainsString('curl_close', $client);
        $this->assertStringContainsString('curl_exec', $client);
    }
}
