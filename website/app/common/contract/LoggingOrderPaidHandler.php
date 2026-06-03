<?php
namespace app\common\contract;

final class LoggingOrderPaidHandler implements OrderPaidHandler
{
    public function onPaid(int $orderId): void
    {
        trace('order paid: ' . $orderId, 'info');
    }
}
