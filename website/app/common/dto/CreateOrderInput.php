<?php
namespace app\common\dto;

final class CreateOrderInput
{
    public function __construct(
        public int $userId,
        public string $outTradeNo,
        public string $protocol,
        public string $channel,
        public string $money,
        public string $productName,
        public string $notifyUrl,
        public string $returnUrl,
        public string $param,
        public string $clientIp,
        public string $floatMode,
        public string $floatStep,
        public string $floatMax,
        public int $timeoutSec,
    ) {
    }
}
