<?php
namespace app\common\repository;

interface CallbackLogRepositoryInterface
{
    public function upsertForOrder(int $orderId, array $data): void;
    public function findByOrder(int $orderId): ?array;
    public function findRetryable(string $now, int $maxAttempts): array;
}
