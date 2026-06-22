<?php
use app\common\dto\CreateOrderInput;
use app\common\exception\ChannelBusyException;
use app\common\service\FloatAmountAllocator;
use app\common\service\OrderCreationService;
use app\common\service\OrderExpirationService;
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
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $order = $service->create(new CreateOrderInput(
            userId: 1,
            outTradeNo: 'T1',
            protocol: 'epay',
            channel: 'wxpay',
            money: '10.00',
            productName: 'test',
            notifyUrl: '',
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
            $locks->tryAcquire(1, 'wxpay', $amount, date('Y-m-d H:i:s', $clock->timestamp() + 300));
        }
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $this->expectException(ChannelBusyException::class);
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', '', '', '', '', 'up', '0.01', '0.01', 300));
    }

    public function test_rejects_notify_url_with_unsupported_scheme(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $this->expectExceptionMessage('回调地址不合法');
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', 'file:///etc/passwd', '', '', '', 'up', '0.01', '0.01', 300));
    }

    public function test_rejects_notify_url_with_private_or_reserved_host(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $this->expectExceptionMessage('回调地址不允许使用内网地址');
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', 'http://127.0.0.1/callback', '', '', '', 'up', '0.01', '0.01', 300));
    }

    public function test_validates_notify_url_target_before_creating_order(): void
    {
        $source = file_get_contents(dirname(__DIR__, 2) . '/app/common/service/OrderCreationService.php') ?: '';

        $this->assertStringContainsString('UrlSafety::assertPublicHttpTarget($url', $source);
    }

    public function test_rejects_return_url_with_unsafe_scheme(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $this->expectExceptionMessage('返回地址不合法');
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', '', 'javascript:alert(1)', '', '', 'up', '0.01', '0.01', 300));
    }

    public function test_rejects_return_url_with_plain_http(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $this->expectExceptionMessage('返回地址必须使用 HTTPS');
        $service->create(new CreateOrderInput(1, 'T1', 'epay', 'wxpay', '10.00', 'test', '', 'http://merchant.test/return', '', '', 'up', '0.01', '0.01', 300));
    }

    public function test_expired_pending_order_does_not_force_next_order_to_float(): void
    {
        $orders = new InMemoryOrderRepository();
        $locks = new InMemoryAmountLockRepository();
        $qrcodes = new InMemoryQrcodeRepository();
        $qrcodes->create(['user_id' => 1, 'channel' => 'wxpay', 'status' => 1]);
        $clock = new FixedClock(1700000000);
        $service = new OrderCreationService($orders, $locks, $qrcodes, new FloatAmountAllocator(), $clock, new OrderExpirationService($orders, $locks, $clock));

        $first = $service->create(new CreateOrderInput(
            userId: 1,
            outTradeNo: 'T1',
            protocol: 'epay',
            channel: 'wxpay',
            money: '0.01',
            productName: 'test',
            notifyUrl: '',
            returnUrl: '',
            param: '',
            clientIp: '127.0.0.1',
            floatMode: 'up',
            floatStep: '0.01',
            floatMax: '0.10',
            timeoutSec: 300,
        ));

        $clock->setTs(1700000601);
        $second = $service->create(new CreateOrderInput(
            userId: 1,
            outTradeNo: 'T2',
            protocol: 'epay',
            channel: 'wxpay',
            money: '0.01',
            productName: 'test',
            notifyUrl: '',
            returnUrl: '',
            param: '',
            clientIp: '127.0.0.1',
            floatMode: 'up',
            floatStep: '0.01',
            floatMax: '0.10',
            timeoutSec: 300,
        ));

        $this->assertSame('expired', $orders->findById((int) $first['id'])['status']);
        $this->assertSame('0.01', $second['real_amount']);
    }
}
