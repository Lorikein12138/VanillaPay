<?php
namespace app\common\service;

use app\common\contract\OrderPaidHandler;
use app\common\dto\MatchResult;
use app\common\dto\PushInput;
use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Clock;

final class PaymentMatcher
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private AmountLockRepositoryInterface $locks,
        private OrderPaidHandler $paidHandler,
        private Clock $clock,
    ) {
    }

    public function match(PushInput $push): MatchResult
    {
        if ($push->tradeNoDevice !== '') {
            $done = $this->orders->findByDeviceTrade($push->userId, $push->tradeNoDevice);
            if ($done) {
                return MatchResult::alreadyDone($done);
            }
        }

        $order = $this->orders->findActivePendingByAmount($push->userId, $push->channel, $push->amountCents, $this->clock->now());
        if (!$order) {
            return MatchResult::unmatched();
        }

        $this->orders->markPaid((int) $order['id'], [
            'status' => 'paid',
            'paid_at' => $this->clock->now(),
            'device_id' => $push->deviceId,
            'device_trade_no' => $push->tradeNoDevice,
        ]);
        $this->locks->release($push->userId, $push->channel, $push->amountCents);
        $this->paidHandler->onPaid((int) $order['id']);

        return MatchResult::matched($this->orders->findById((int) $order['id']));
    }
}
