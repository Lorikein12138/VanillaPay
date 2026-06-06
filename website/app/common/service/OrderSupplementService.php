<?php
namespace app\common\service;

use app\common\contract\OrderPaidHandler;
use app\common\exception\ValidationException;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Clock;

final class OrderSupplementService
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private OrderPaidHandler $paidHandler,
        private Clock $clock
    )
    {
    }

    public function supplement(int $userId, int $orderId): array
    {
        $order = $this->orders->findById($orderId);
        if (!$order || (int) ($order['user_id'] ?? 0) !== $userId) {
            throw new ValidationException('订单不存在');
        }

        $status = (string) ($order['status'] ?? '');
        if ($status === 'paid') {
            throw new ValidationException('已支付订单不可补单');
        }
        if (!in_array($status, ['pending', 'expired'], true)) {
            throw new ValidationException('当前订单状态不可补单');
        }

        $this->orders->markPaid($orderId, [
            'status' => 'paid',
            'paid_at' => $this->clock->now(),
        ]);

        $callbackDispatched = true;
        $callbackError = '';
        try {
            $this->paidHandler->onPaid($orderId);
        } catch (\Throwable $e) {
            $callbackDispatched = false;
            $callbackError = $e->getMessage();
        }

        $updated = $this->orders->findById($orderId) ?? $order;
        $updated['callback_dispatched'] = $callbackDispatched;
        if (!$callbackDispatched) {
            $updated['callback_error'] = $callbackError;
        }

        return $updated;
    }
}
