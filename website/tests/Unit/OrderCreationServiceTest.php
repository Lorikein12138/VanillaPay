<?php
use app\common\dto\CreateOrderInput;
use app\common\exception\ChannelBusyException;
use app\common\service\FloatAmountAllocator;
use app\common\service\OrderCreationService;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
use tests\Support\InMemoryAmountLockRepository;
use tests\Support\InMemoryOrderRepository;
use tests\Support\InMemoryQrcodeRepository;

final class OrderCreationServiceTest extends TestCase
{
    public function test_creates_pending_order_and_amount_lock(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1, 'qr_image_path' => '/q.png']);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), new FixedClock(1700000000));

        $order = $service->create(new CreateOrderInput(
            userId: 1,
            outTradeNo: 'T1',
            protocol: 'epay',
            channel: 'wxpay',
            money: '10.00',
            productName: 'test',
            notifyUrl: 'https://merchant.test/n',
            returnUrl: '',
            param: '',
            clientIp: '127.0.0.1',
            floatMode: 'up',
            floatStep: '0.01',
            floatMax: '0.03',
            timeoutSec: 300,
        ));

        $this->assertSame('pending', $order['status']);
        $this->assertSame('10.00', $order['real_amount']);
        $this->assertNotEmpty($locks->locks);
    }

    public function test_busy_when_all_candidate_amounts_locked(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        foreach ([1000, 1001] as $amount) {
            $locks->tryAcquire(1, 'wxpay', $amount, '2023-11-14 22:18:20');
        }
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock);

        $this->expectException(ChannelBusyException::class);
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', '', '', '', '', 'up', '0.01', '0.01', 300));
    }
}
