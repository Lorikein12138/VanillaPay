<?php
use app\common\service\RateLimiter;
use PHPUnit\Framework\TestCase;
use tests\Support\ArrayCacheStore;

final class RateLimiterTest extends TestCase
{
    public function test_allows_until_limit_then_blocks(): void
    {
        $limiter = new RateLimiter(new ArrayCacheStore());
        $this->assertTrue($limiter->allow('login:127.0.0.1', 2, 60));
        $this->assertTrue($limiter->allow('login:127.0.0.1', 2, 60));
        $this->assertFalse($limiter->allow('login:127.0.0.1', 2, 60));
    }
}
