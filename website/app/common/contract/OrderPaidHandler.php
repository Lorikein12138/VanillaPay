<?php
namespace app\common\contract;

interface OrderPaidHandler
{
    public function onPaid(int $orderId): void;
}
