<?php
namespace app\common\protocol;

use app\common\dto\ParsedOrder;

interface PayProtocolAdapter
{
    public function code(): string;
    public function verifyOrderSign(array $params, string $apiKey): bool;
    public function parseOrder(array $params): ParsedOrder;
    public function buildNotifyParams(array $order, string $pid, string $apiKey): array;
    public function buildReturnParams(array $order, string $pid, string $apiKey): array;
    public function successText(): string;
}
