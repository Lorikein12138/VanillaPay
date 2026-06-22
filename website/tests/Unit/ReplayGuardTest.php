<?php
use app\common\service\ReplayGuard;
use app\common\support\CacheStore;
use PHPUnit\Framework\TestCase;
use tests\Support\ArrayCacheStore;

final class ReplayGuardTest extends TestCase
{
    public function test_first_use_ok_second_is_replay(): void
    {
        $guard = new ReplayGuard(new ArrayCacheStore());
        $this->assertTrue($guard->firstUse('dev1:sign', 300));
        $this->assertFalse($guard->firstUse('dev1:sign', 300));
    }

    public function test_first_use_uses_single_cache_claim_operation(): void
    {
        $cache = new class implements CacheStore {
            public array $claimed = [];
            public bool $usedClaim = false;

            public function increment(string $key, int $ttlSeconds): int
            {
                return 1;
            }

            public function has(string $key): bool
            {
                throw new RuntimeException('ReplayGuard must not check then write replay keys');
            }

            public function put(string $key, string $value, int $ttlSeconds): void
            {
                throw new RuntimeException('ReplayGuard must not check then write replay keys');
            }

            public function putIfAbsent(string $key, string $value, int $ttlSeconds): bool
            {
                $this->usedClaim = true;
                if (isset($this->claimed[$key])) {
                    return false;
                }
                $this->claimed[$key] = $value;
                return true;
            }
        };

        $guard = new ReplayGuard($cache);

        $this->assertTrue($guard->firstUse('dev1:sign', 300));
        $this->assertFalse($guard->firstUse('dev1:sign', 300));
        $this->assertTrue($cache->usedClaim);
    }
}
