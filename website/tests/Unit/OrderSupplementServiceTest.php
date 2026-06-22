<?php
use app\common\contract\OrderPaidHandler;
use app\common\exception\ValidationException;
use app\common\service\OrderSupplementService;
use app\common\support\Money;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryOrderRepository;

final class OrderSupplementServiceTest extends TestCase
{
    public function test_supplements_pending_order_and_dispatches_paid_callback(): void
    {
        $orders = new InMemoryOrderRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public array $ids = [];
            public function onPaid(int $orderId): void { $this->ids[] = $orderId; }
        };
        $locks = new InMemoryAmountLockRepository();
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'paid_at' => null,
        ]);

        $service = new OrderSupplementService($orders, $locks, $handler, $clock);
        $order = $service->supplement(10, $id);

        $this->assertSame('paid', $order['status']);
        $this->assertTrue($order['callback_dispatched']);
        $this->assertSame($clock->now(), $orders->findById($id)['paid_at']);
        $this->assertSame([$id], $handler->ids);
    }

    public function test_supplements_expired_order_and_dispatches_paid_callback(): void
    {
        $orders = new InMemoryOrderRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public array $ids = [];
            public function onPaid(int $orderId): void { $this->ids[] = $orderId; }
        };
        $locks = new InMemoryAmountLockRepository();
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'expired',
            'paid_at' => null,
        ]);

        $service = new OrderSupplementService($orders, $locks, $handler, $clock);
        $service->supplement(10, $id);

        $this->assertSame('paid', $orders->findById($id)['status']);
        $this->assertSame([$id], $handler->ids);
    }

    public function test_supplements_pending_order_and_releases_amount_lock(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public function onPaid(int $orderId): void {}
        };
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'channel' => 'wxpay',
            'real_amount' => '10.00',
            'paid_at' => null,
        ]);
        $locks->tryAcquire(10, 'wxpay', Money::toCents('10.00'), date('Y-m-d H:i:s', $clock->timestamp() + 300));
        $locks->attachOrder(10, 'wxpay', Money::toCents('10.00'), $id);

        (new OrderSupplementService($orders, $locks, $handler, $clock))->supplement(10, $id);

        $this->assertSame([], $locks->locks);
    }

    public function test_supplement_keeps_order_paid_when_callback_dispatch_throws(): void
    {
        $orders = new InMemoryOrderRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public function onPaid(int $orderId): void { throw new RuntimeException('notify failed'); }
        };
        $locks = new InMemoryAmountLockRepository();
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'paid_at' => null,
        ]);

        $order = (new OrderSupplementService($orders, $locks, $handler, $clock))->supplement(10, $id);

        $this->assertSame('paid', $orders->findById($id)['status']);
        $this->assertSame($clock->now(), $orders->findById($id)['paid_at']);
        $this->assertFalse($order['callback_dispatched']);
    }

    public function test_rejects_paid_order_without_dispatching_callback(): void
    {
        $orders = new InMemoryOrderRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public array $ids = [];
            public function onPaid(int $orderId): void { $this->ids[] = $orderId; }
        };
        $locks = new InMemoryAmountLockRepository();
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'paid',
            'paid_at' => $clock->now(),
        ]);

        $this->expectException(ValidationException::class);
        try {
            (new OrderSupplementService($orders, $locks, $handler, $clock))->supplement(10, $id);
        } finally {
            $this->assertSame([], $handler->ids);
        }
    }
}
