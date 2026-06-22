<?php
namespace app\common\contract;

use app\common\service\CallbackSender;

final class CallbackDispatcher implements OrderPaidHandler
{
    public function __construct(private CallbackSender $sender)
    {
    }

    public function onPaid(int $orderId): void
    {
        $this->sender->sendForOrder($orderId);
    }
}
