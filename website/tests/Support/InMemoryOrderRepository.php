<?php
namespace tests\Support;

use app\common\repository\OrderRepositoryInterface;
use app\common\support\Money;

final class InMemoryOrderRepository implements OrderRepositoryInterface
{
    /** @var array<int,array> */
    public array $rows = [];
    private int $auto = 0;

    public function create(array $data): int
    {
        $id = ++$this->auto;
        $this->rows[$id] = ['id' => $id] + $data;
        return $id;
    }

    public function findById(int $id): ?array
    {
        return $this->rows[$id] ?? null;
    }

    public function findByOrderNo(string $orderNo): ?array
    {
        foreach ($this->rows as $row) {
            if (($row['order_no'] ?? '') === $orderNo) {
                return $row;
            }
        }
        return null;
    }

    public function findByUserOutTradeNo(int $userId, string $outTradeNo): ?array
    {
        foreach ($this->rows as $row) {
            if ((int) $row['user_id'] === $userId && ($row['out_trade_no'] ?? '') === $outTradeNo) {
                return $row;
            }
        }
        return null;
    }

    public function findActivePendingByAmount(int $userId, string $channel, int $amountCents, string $now): ?array
    {
        foreach ($this->rows as $row) {
            if ((int) $row['user_id'] === $userId
                && ($row['channel'] ?? '') === $channel
                && ($row['status'] ?? '') === 'pending'
                && Money::toCents($row['real_amount'] ?? '0') === $amountCents
                && (empty($row['expire_at']) || $row['expire_at'] > $now)) {
                return $row;
            }
        }
        return null;
    }

    public function findByDeviceTrade(int $userId, string $deviceTradeNo): ?array
    {
        if ($deviceTradeNo === '') {
            return null;
        }
        foreach ($this->rows as $row) {
            if ((int) $row['user_id'] === $userId && ($row['device_trade_no'] ?? '') === $deviceTradeNo) {
                return $row;
            }
        }
        return null;
    }

    public function markPaid(int $id, array $data): void
    {
        $this->rows[$id] = array_merge($this->rows[$id], $data);
    }

    public function markExpiredBatch(string $now): int
    {
        $count = 0;
        foreach ($this->rows as $id => $row) {
            if (($row['status'] ?? '') === 'pending' && !empty($row['expire_at']) && $row['expire_at'] <= $now) {
                $this->rows[$id]['status'] = 'expired';
                $count++;
            }
        }
        return $count;
    }

    public function update(int $id, array $data): void
    {
        $this->rows[$id] = array_merge($this->rows[$id] ?? ['id' => $id], $data);
    }

    public function paginateByUser(int $userId, array $filters, int $page, int $pageSize): array
    {
        $items = array_values(array_filter($this->rows, fn (array $row): bool => (int) $row['user_id'] === $userId));
        return ['items' => $items, 'total' => count($items), 'page' => $page, 'page_size' => $pageSize];
    }

    public function paginateAll(array $filters, int $page, int $pageSize): array
    {
        return ['items' => array_values($this->rows), 'total' => count($this->rows), 'page' => $page, 'page_size' => $pageSize];
    }

    public function countByStatusBetween(string $status, string $start, string $end): int
    {
        return count(array_filter($this->rows, fn (array $row): bool => ($row['status'] ?? '') === $status
            && ($row['create_time'] ?? '') >= $start && ($row['create_time'] ?? '') <= $end));
    }

    public function sumPaidBetween(string $start, string $end): string
    {
        $cents = 0;
        foreach ($this->rows as $row) {
            if (($row['status'] ?? '') === 'paid' && ($row['paid_at'] ?? '') >= $start && ($row['paid_at'] ?? '') <= $end) {
                $cents += Money::toCents($row['real_amount'] ?? '0');
            }
        }
        return Money::fromCents($cents);
    }

    public function countNotifyFailBetween(string $start, string $end): int
    {
        return count(array_filter($this->rows, fn (array $row): bool => (int) ($row['notify_status'] ?? 0) === 2
            && ($row['create_time'] ?? '') >= $start && ($row['create_time'] ?? '') <= $end));
    }
}
