<?php

use PHPUnit\Framework\TestCase;
use tests\Support\InMemoryOrderRepository;

final class OrderDashboardMetricRepositoryTest extends TestCase
{
    public function testSumsPaidAmountsByMerchantStatusAndChannel(): void
    {
        $orders = new InMemoryOrderRepository();
        $orders->create(['user_id' => 1, 'status' => 'paid', 'channel' => 'wxpay', 'real_amount' => '1.20']);
        $orders->create(['user_id' => 1, 'status' => 'paid', 'channel' => 'alipay', 'real_amount' => '2.30']);
        $orders->create(['user_id' => 1, 'status' => 'pending', 'channel' => 'wxpay', 'real_amount' => '9.99']);
        $orders->create(['user_id' => 2, 'status' => 'paid', 'channel' => 'wxpay', 'real_amount' => '8.88']);

        $this->assertSame('3.50', $orders->sumByUser(1, ['status' => 'paid']));
        $this->assertSame('1.20', $orders->sumByUser(1, ['status' => 'paid', 'channel' => 'wxpay']));
        $this->assertSame('2.30', $orders->sumByUser(1, ['status' => 'paid', 'channel' => 'alipay']));
    }
}
