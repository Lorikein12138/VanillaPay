<?php

use PHPUnit\Framework\TestCase;
use tests\Support\InMemoryOrderRepository;

final class OrderCleanupRepositoryTest extends TestCase
{
    public function testDeletesOnlyExpiredOrdersForCurrentMerchant(): void
    {
        $orders = new InMemoryOrderRepository();
        $expiredCurrent = $orders->create(['user_id' => 1, 'status' => 'expired', 'order_no' => 'E1']);
        $paidCurrent = $orders->create(['user_id' => 1, 'status' => 'paid', 'order_no' => 'P1']);
        $expiredOther = $orders->create(['user_id' => 2, 'status' => 'expired', 'order_no' => 'E2']);

        $deleted = $orders->deleteExpiredByUser(1);

        $this->assertSame(1, $deleted);
        $this->assertNull($orders->findById($expiredCurrent));
        $this->assertNotNull($orders->findById($paidCurrent));
        $this->assertNotNull($orders->findById($expiredOther));
    }
}
