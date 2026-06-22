<?php
use PHPUnit\Framework\TestCase;
use app\common\service\CredentialGenerator;

final class CredentialGeneratorTest extends TestCase
{
    public function test_pid_is_numeric_and_length_in_range(): void
    {
        $pid = (new CredentialGenerator())->pid();
        $this->assertMatchesRegularExpression('/^\d{6,12}$/', $pid);
    }

    public function test_api_key_is_32_hex_and_unique(): void
    {
        $g = new CredentialGenerator();
        $a = $g->apiKey();
        $b = $g->apiKey();
        $this->assertMatchesRegularExpression('/^[a-f0-9]{32}$/', $a);
        $this->assertNotSame($a, $b);
    }
}
