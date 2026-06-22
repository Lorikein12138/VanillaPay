<?php
use PHPUnit\Framework\TestCase;
use app\common\service\PasswordHasher;

final class PasswordHasherTest extends TestCase
{
    public function test_hash_then_verify_succeeds(): void
    {
        $h = new PasswordHasher();
        $hash = $h->hash('S3cret!');
        $this->assertNotSame('S3cret!', $hash);
        $this->assertTrue($h->verify('S3cret!', $hash));
    }

    public function test_verify_rejects_wrong_password(): void
    {
        $h = new PasswordHasher();
        $this->assertFalse($h->verify('wrong', $h->hash('S3cret!')));
    }
}
