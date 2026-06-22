<?php

use app\common\service\OrderExpirationService;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryOrderRepository;

final class OrderExpirationServiceTest extends TestCase
{
    public function testRefreshExpiresTimedOutOrdersAndReleasesExpiredLocks(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $clock = new FixedClock(1700000000);
        $expiredAt = date('Y-m-d H:i:s', $clock->timestamp() - 60);
        $activeAt = date('Y-m-d H:i:s', $clock->timestamp() + 300);
        $expiredOrder = $orders->create([
            'user_id' => 1,
            'status' => 'pending',
            'expire_at' => $expiredAt,
        ]);
        $activeOrder = $orders->create([
            'user_id' => 1,
            'status' => 'pending',
            'expire_at' => $activeAt,
        ]);
        $locks->tryAcquire(1, 'wxpay', 1, $expiredAt);
        $locks->tryAcquire(1, 'wxpay', 2, $activeAt);

        $result = (new OrderExpirationService($orders, $locks, $clock))->refresh();

        $this->assertSame(['orders' => 1, 'locks' => 1], $result);
        $this->assertSame('expired', $orders->findById($expiredOrder)['status']);
        $this->assertSame('pending', $orders->findById($activeOrder)['status']);
        $this->assertCount(1, $locks->locks);
    }
}
