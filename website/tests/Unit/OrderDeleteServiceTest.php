<?php
use app\common\exception\ValidationException;
use app\common\service\OrderDeleteService;
use PHPUnit\Framework\TestCase;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryOrderRepository;

final class OrderDeleteServiceTest extends TestCase
{
    public function testDeletesPendingOrderAndReleasesAmountLock(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $locks->tryAcquire(10, 'wxpay', 1001, '2099-01-01 00:00:00');
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'channel' => 'wxpay',
            'real_amount' => '10.01',
        ]);

        (new OrderDeleteService($orders, $locks))->deleteForUser(10, $id);

        $this->assertNull($orders->findById($id));
        $this->assertSame([], $locks->locks);
    }

    public function testRejectsDeletingAnotherMerchantOrder(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $id = $orders->create([
            'user_id' => 20,
            'status' => 'pending',
            'channel' => 'alipay',
            'real_amount' => '1.00',
        ]);

        $this->expectException(ValidationException::class);
        try {
            (new OrderDeleteService($orders, $locks))->deleteForUser(10, $id);
        } finally {
            $this->assertNotNull($orders->findById($id));
        }
    }
}
