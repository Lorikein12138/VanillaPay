<?php
namespace app\common\service;

use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Clock;

final class OrderExpirationService
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private AmountLockRepositoryInterface $locks,
        private Clock $clock,
    ) {
    }

    /**
     * @return array{orders:int,locks:int}
     */
    public function refresh(): array
    {
        $now = $this->clock->now();

        return [
            'orders' => $this->orders->markExpiredBatch($now),
            'locks' => $this->locks->releaseExpired($now),
        ];
    }
}
