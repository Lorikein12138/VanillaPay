<?php
use app\common\contract\OrderPaidHandler;
use app\common\dto\PushInput;
use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\service\DeviceSigner;
use app\common\service\PaymentMatcher;
use app\common\service\ReplayGuard;
use app\common\support\Clock;
use PHPUnit\Framework\TestCase;
use tests\Support\ArrayCacheStore;
use tests\Support\FixedClock;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryDeviceRepository;
use tests\Support\InMemoryOrderRepository;
use tests\Support\InMemoryRiskEventRepository;

final class DevicePushStatusTest extends TestCase
{
    public function testDevicePushResponseIncludesUnmatchedStatusForRetryDecision(): void
    {
        $source = file_get_contents(dirname(__DIR__, 2) . '/app/device/controller/Push.php') ?: '';

        $this->assertStringContainsString("'status' => \$result->status", $source);
    }

    public function testDuplicatePushResponseUsesAlreadyDoneStatus(): void
    {
        $source = file_get_contents(dirname(__DIR__, 2) . '/app/device/controller/Push.php') ?: '';

        $this->assertStringContainsString("'status' => MatchResult::ALREADY_DONE", $source);
    }

    public function testMatchedFlagTreatsAlreadyDoneAsSuccessfulMatch(): void
    {
        $clock = new FixedClock(1700000000);
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $handler = new class implements OrderPaidHandler {
            public function onPaid(int $orderId): void {}
        };
        $orders->create([
            'order_no' => 'ORD',
            'user_id' => 1,
            'channel' => 'wxpay',
            'real_amount' => '10.00',
            'status' => 'paid',
            'expire_at' => date('Y-m-d H:i:s', $clock->timestamp() + 400),
            'device_trade_no' => 'N1',
        ]);

        $matcher = new PaymentMatcher($orders, $locks, $handler, $clock);
        $result = $matcher->match(new PushInput(1, 9, 'wxpay', 1000, 'N1'));

        $this->assertTrue($result->isAlreadyDone());
        $this->assertTrue($result->isSettled());
    }
}
