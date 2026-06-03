<?php
use PHPUnit\Framework\TestCase;
use app\common\service\PasswordResetService;

final class PasswordResetServiceTest extends TestCase
{
    public function test_token_round_trips_within_ttl(): void
    {
        $s = new PasswordResetService('app-secret', ttl: 3600);
        $token = $s->issue(42);
        $this->assertSame(42, $s->verify($token));
    }

    public function test_tampered_token_rejected(): void
    {
        $s = new PasswordResetService('app-secret', ttl: 3600);
        $this->assertNull($s->verify($s->issue(42) . 'x'));
    }

    public function test_expired_token_rejected(): void
    {
        $s = new PasswordResetService('app-secret', ttl: -1);
        $this->assertNull($s->verify($s->issue(42)));
    }

    public function test_wrong_secret_rejected(): void
    {
        $token = (new PasswordResetService('secret-A', ttl: 3600))->issue(42);
        $this->assertNull((new PasswordResetService('secret-B', ttl: 3600))->verify($token));
    }
}
