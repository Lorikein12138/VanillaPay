<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Money;

final class OrderDeleteService
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private AmountLockRepositoryInterface $locks,
    )
    {
    }

    public function deleteForUser(int $userId, int $orderId): void
    {
        $order = $this->orders->findById($orderId);
        if (!$order || (int) ($order['user_id'] ?? 0) !== $userId) {
            throw new ValidationException('订单不存在');
        }

        if (in_array((string) ($order['status'] ?? ''), ['pending', 'expired'], true)) {
            $this->locks->release(
                $userId,
                (string) ($order['channel'] ?? ''),
                Money::toCents((string) ($order['real_amount'] ?? '0'))
            );
        }

        if ($this->orders->deleteForUser($orderId, $userId) < 1) {
            throw new ValidationException('订单不存在');
        }
    }
}
