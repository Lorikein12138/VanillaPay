<?php

use PHPUnit\Framework\TestCase;
use tests\Support\InMemoryOrderRepository;

final class OrderDashboardMetricRepositoryTest extends TestCase
{
    public function testBuildsMerchantDashboardMetricsWithGroupedCountsAndAmounts(): void
    {
        $orders = new InMemoryOrderRepository();
        $orders->create(['user_id' => 1, 'status' => 'paid', 'channel' => 'wxpay', 'real_amount' => '1.20']);
        $orders->create(['user_id' => 1, 'status' => 'paid', 'channel' => 'alipay', 'real_amount' => '2.30']);
        $orders->create(['user_id' => 1, 'status' => 'pending', 'channel' => 'wxpay', 'real_amount' => '9.99']);
        $orders->create(['user_id' => 1, 'status' => 'expired', 'channel' => 'alipay', 'real_amount' => '3.00']);
        $orders->create(['user_id' => 2, 'status' => 'paid', 'channel' => 'wxpay', 'real_amount' => '8.88']);

        $this->assertSame([
            'totalOrders' => 4,
            'paidOrders' => 2,
            'pendingOrders' => 1,
            'expiredOrders' => 1,
            'paidAmount' => '3.50',
            'paidAlipayAmount' => '2.30',
            'paidWxpayAmount' => '1.20',
        ], $orders->dashboardMetricsByUser(1));
    }

    public function testBuildsConsoleDashboardMetricsAcrossAllMerchants(): void
    {
        $orders = new InMemoryOrderRepository();
        $orders->create(['user_id' => 1, 'status' => 'paid', 'channel' => 'wxpay', 'real_amount' => '1.20']);
        $orders->create(['user_id' => 1, 'status' => 'pending', 'channel' => 'wxpay', 'real_amount' => '9.99']);
        $orders->create(['user_id' => 2, 'status' => 'paid', 'channel' => 'alipay', 'real_amount' => '8.88']);
        $orders->create(['user_id' => 2, 'status' => 'expired', 'channel' => 'alipay', 'real_amount' => '3.00']);

        $this->assertSame([
            'totalOrders' => 4,
            'paidOrders' => 2,
            'pendingOrders' => 1,
            'expiredOrders' => 1,
            'paidAmount' => '10.08',
            'paidAlipayAmount' => '8.88',
            'paidWxpayAmount' => '1.20',
        ], $orders->dashboardMetricsAll());
    }

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
