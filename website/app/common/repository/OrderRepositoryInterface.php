<?php
namespace app\common\repository;

interface OrderRepositoryInterface
{
    public function create(array $data): int;
    public function findById(int $id): ?array;
    public function findByOrderNo(string $orderNo): ?array;
    public function findByUserOutTradeNo(int $userId, string $outTradeNo): ?array;
    public function findActivePendingByAmount(int $userId, string $channel, int $amountCents, string $now): ?array;
    public function findByDeviceTrade(int $userId, string $deviceTradeNo): ?array;
    public function markPaid(int $id, array $data): void;
    public function markPendingPaid(int $id, array $data): bool;
    public function closePending(int $id): bool;
    public function markExpiredBatch(string $now): int;
    public function deleteExpiredByUser(int $userId): int;
    public function deleteForUser(int $id, int $userId): int;
    public function update(int $id, array $data): void;
    public function paginateByUser(int $userId, array $filters, int $page, int $pageSize): array;
    public function sumByUser(int $userId, array $filters): string;
    public function dashboardMetricsByUser(int $userId): array;
    public function dashboardMetricsAll(): array;
    public function paginateAll(array $filters, int $page, int $pageSize): array;
    public function countByStatusBetween(string $status, string $start, string $end): int;
    public function sumPaidBetween(string $start, string $end): string;
    public function countNotifyFailBetween(string $start, string $end): int;
}
