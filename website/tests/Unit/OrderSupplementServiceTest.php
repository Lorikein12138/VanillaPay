<?php
use app\common\contract\OrderPaidHandler;
use app\common\exception\ValidationException;
use app\common\service\OrderSupplementService;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
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
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'paid_at' => null,
        ]);

        $service = new OrderSupplementService($orders, $handler, $clock);
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
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'expired',
            'paid_at' => null,
        ]);

        $service = new OrderSupplementService($orders, $handler, $clock);
        $service->supplement(10, $id);

        $this->assertSame('paid', $orders->findById($id)['status']);
        $this->assertSame([$id], $handler->ids);
    }

    public function test_supplement_keeps_order_paid_when_callback_dispatch_throws(): void
    {
        $orders = new InMemoryOrderRepository();
        $clock = new FixedClock(1700000000);
        $handler = new class implements OrderPaidHandler {
            public function onPaid(int $orderId): void { throw new RuntimeException('notify failed'); }
        };
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'pending',
            'paid_at' => null,
        ]);

        $order = (new OrderSupplementService($orders, $handler, $clock))->supplement(10, $id);

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
        $id = $orders->create([
            'user_id' => 10,
            'status' => 'paid',
            'paid_at' => $clock->now(),
        ]);

        $this->expectException(ValidationException::class);
        try {
            (new OrderSupplementService($orders, $handler, $clock))->supplement(10, $id);
        } finally {
            $this->assertSame([], $handler->ids);
        }
    }
}
