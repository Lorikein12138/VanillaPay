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

    public function testDeletesSingleOrderForCurrentMerchantOnly(): void
    {
        $orders = new InMemoryOrderRepository();
        $current = $orders->create(['user_id' => 1, 'status' => 'pending', 'order_no' => 'D1']);
        $other = $orders->create(['user_id' => 2, 'status' => 'pending', 'order_no' => 'D2']);

        $this->assertSame(0, $orders->deleteForUser($other, 1));
        $this->assertSame(1, $orders->deleteForUser($current, 1));

        $this->assertNull($orders->findById($current));
        $this->assertNotNull($orders->findById($other));
    }

    public function testPaginateByUserFiltersOrderFieldsChannelStatusAndSlicesPage(): void
    {
        $orders = new InMemoryOrderRepository();
        for ($i = 1; $i <= 12; $i++) {
            $orders->create([
                'user_id' => 1,
                'status' => $i === 12 ? 'paid' : 'pending',
                'channel' => $i % 2 === 0 ? 'wxpay' : 'alipay',
                'order_no' => sprintf('NO%02d', $i),
                'out_trade_no' => sprintf('MERCHANT%02d', $i),
            ]);
        }
        $orders->create(['user_id' => 2, 'status' => 'pending', 'channel' => 'wxpay', 'order_no' => 'NO99', 'out_trade_no' => 'MERCHANT99']);

        $pageOne = $orders->paginateByUser(1, ['status' => 'pending'], 1, 10);
        $pageTwo = $orders->paginateByUser(1, ['status' => 'pending'], 2, 10);
        $filtered = $orders->paginateByUser(1, [
            'order_no' => 'NO10',
            'out_trade_no' => 'MERCHANT10',
            'channel' => 'wxpay',
            'status' => 'pending',
        ], 1, 10);

        $this->assertSame(11, $pageOne['total']);
        $this->assertCount(10, $pageOne['items']);
        $this->assertCount(1, $pageTwo['items']);
        $this->assertSame(['NO10'], array_column($filtered['items'], 'order_no'));
    }
}
