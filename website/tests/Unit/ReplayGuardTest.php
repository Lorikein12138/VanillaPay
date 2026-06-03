<?php
use app\common\service\ReplayGuard;
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
}
