<?php
namespace app\common\repository;

interface AmountLockRepositoryInterface
{
    public function tryAcquire(int $userId, string $channel, int $amountCents, string $expireAt): bool;
    public function attachOrder(int $userId, string $channel, int $amountCents, int $orderId): void;
    public function release(int $userId, string $channel, int $amountCents): void;
    public function releaseExpired(string $now): int;
}
