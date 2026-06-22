<?php
namespace app\common\dto;

final class PushInput
{
    public function __construct(
        public int $userId,
        public int $deviceId,
        public string $channel,
        public int $amountCents,
        public string $tradeNoDevice,
    ) {
    }
}
