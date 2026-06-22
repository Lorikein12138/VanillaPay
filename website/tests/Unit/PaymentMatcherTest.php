<?php
use app\common\contract\OrderPaidHandler;
use app\common\dto\PushInput;
use app\common\service\PaymentMatcher;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryOrderRepository;

final class PaymentMatcherTest extends TestCase
{
    public function test_matches_pending_order_by_amount_and_releases_lock(): void
    {
        $clock = new FixedClock(1700000000);
        $expireAt = date('Y-m-d H:i:s', $clock->timestamp() + 400);
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $handler = new class implements OrderPaidHandler {
            public array $ids = [];
            public function onPaid(int $orderId): void { $this->ids[] = $orderId; }
        };
        $orders->create([
            'order_no' => 'ORD',
            'user_id' => 1,
            'channel' => 'wxpay',
            'real_amount' => '10.00',
            'status' => 'pending',
            'expire_at' => $expireAt,
        ]);
        $locks->tryAcquire(1, 'wxpay', 1000, $expireAt);
        $matcher = new PaymentMatcher($orders, $locks, $handler, $clock);

        $result = $matcher->match(new PushInput(1, 9, 'wxpay', 1000, 'N1'));

        $this->assertTrue($result->isMatched());
        $this->assertSame('paid', $orders->findById(1)['status']);
        $this->assertSame([1], $handler->ids);
        $this->assertSame([], $locks->locks);
    }

    public function test_does_not_dispatch_callback_when_pending_order_was_paid_by_concurrent_match(): void
    {
        $clock = new FixedClock(1700000000);
        $expireAt = date('Y-m-d H:i:s', $clock->timestamp() + 400);
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $handler = new class implements OrderPaidHandler {
            public array $ids = [];
            public function onPaid(int $orderId): void { $this->ids[] = $orderId; }
        };
        $id = $orders->create([
            'order_no' => 'ORD',
            'user_id' => 1,
            'channel' => 'wxpay',
            'real_amount' => '10.00',
            'status' => 'pending',
            'expire_at' => $expireAt,
        ]);
        $orders->rejectPendingPaid = true;

        $matcher = new PaymentMatcher($orders, $locks, $handler, $clock);
        $result = $matcher->match(new PushInput(1, 9, 'wxpay', 1000, 'N1'));

        $this->assertTrue($result->isUnmatched());
        $this->assertSame([], $handler->ids);
    }
}
