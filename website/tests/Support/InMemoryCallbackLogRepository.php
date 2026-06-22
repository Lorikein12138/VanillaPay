<?php
namespace tests\Support;

use app\common\repository\CallbackLogRepositoryInterface;

final class InMemoryCallbackLogRepository implements CallbackLogRepositoryInterface
{
    /** @var array<int,array> */
    public array $rows = [];

    public function upsertForOrder(int $orderId, array $data): void
    {
        $this->rows[$orderId] = array_merge($this->rows[$orderId] ?? ['order_id' => $orderId], $data);
    }

    public function findByOrder(int $orderId): ?array
    {
        return $this->rows[$orderId] ?? null;
    }

    public function findRetryable(string $now, int $maxAttempts): array
    {
        return array_values(array_filter($this->rows, fn (array $row): bool => empty($row['success'])
            && (int) ($row['attempts'] ?? 0) < $maxAttempts
            && !empty($row['next_retry_at'])
            && $row['next_retry_at'] <= $now));
    }
}
