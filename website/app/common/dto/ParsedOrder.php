<?php
namespace app\common\dto;

final class ParsedOrder
{
    public function __construct(
        public string $pid,
        public string $outTradeNo,
        public string $channel,
        public string $money,
        public string $productName,
        public string $notifyUrl,
        public string $returnUrl,
        public string $param,
    ) {
    }
}
