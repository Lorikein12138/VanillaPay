<?php
namespace app\common\repository;

use app\common\support\Money;
use think\facade\Db;

class ThinkOrderRepository implements OrderRepositoryInterface
{
    private function table()
    {
        return Db::name('orders');
    }

    public function create(array $data): int
    {
        return (int) $this->table()->insertGetId($data);
    }

    public function findById(int $id): ?array
    {
        return $this->table()->where('id', $id)->find();
    }

    public function findByOrderNo(string $orderNo): ?array
    {
        return $this->table()->where('order_no', $orderNo)->find();
    }

    public function findByUserOutTradeNo(int $userId, string $outTradeNo): ?array
    {
        return $this->table()->where('user_id', $userId)->where('out_trade_no', $outTradeNo)->find();
    }

    public function findActivePendingByAmount(int $userId, string $channel, int $amountCents, string $now): ?array
    {
        return $this->table()
            ->where('user_id', $userId)
            ->where('channel', $channel)
            ->where('real_amount', Money::fromCents($amountCents))
            ->where('status', 'pending')
            ->where('expire_at', '>', $now)
            ->find();
    }

    public function findByDeviceTrade(int $userId, string $deviceTradeNo): ?array
    {
        if ($deviceTradeNo === '') {
            return null;
        }
        return $this->table()->where('user_id', $userId)->where('device_trade_no', $deviceTradeNo)->find();
    }

    public function markPaid(int $id, array $data): void
    {
        $this->table()->where('id', $id)->update($data);
    }

    public function markExpiredBatch(string $now): int
    {
        return (int) $this->table()
            ->where('status', 'pending')
            ->where('expire_at', '<=', $now)
            ->update(['status' => 'expired']);
    }

    public function update(int $id, array $data): void
    {
        $this->table()->where('id', $id)->update($data);
    }

    public function paginateByUser(int $userId, array $filters, int $page, int $pageSize): array
    {
        $query = $this->table()->where('user_id', $userId);
        $this->applyFilters($query, $filters);
        $total = (int) (clone $query)->count();
        $items = $query->order('id', 'desc')->page($page, $pageSize)->select()->toArray();
        return ['items' => $items, 'total' => $total, 'page' => $page, 'page_size' => $pageSize];
    }

    public function paginateAll(array $filters, int $page, int $pageSize): array
    {
        $query = $this->table();
        $this->applyFilters($query, $filters);
        $total = (int) (clone $query)->count();
        $items = $query->order('id', 'desc')->page($page, $pageSize)->select()->toArray();
        return ['items' => $items, 'total' => $total, 'page' => $page, 'page_size' => $pageSize];
    }

    private function applyFilters($query, array $filters): void
    {
        if (($filters['status'] ?? '') !== '') {
            $query->where('status', (string) $filters['status']);
        }
        if (($filters['user_id'] ?? '') !== '') {
            $query->where('user_id', (int) $filters['user_id']);
        }
        if (($filters['keyword'] ?? '') !== '') {
            $keyword = '%' . trim((string) $filters['keyword']) . '%';
            $query->whereLike('order_no|out_trade_no', $keyword);
        }
    }

    public function countByStatusBetween(string $status, string $start, string $end): int
    {
        return (int) $this->table()->where('status', $status)->whereBetween('create_time', [$start, $end])->count();
    }

    public function sumPaidBetween(string $start, string $end): string
    {
        $sum = $this->table()->where('status', 'paid')->whereBetween('paid_at', [$start, $end])->sum('real_amount');
        return number_format((float) $sum, 2, '.', '');
    }

    public function countNotifyFailBetween(string $start, string $end): int
    {
        return (int) $this->table()->where('notify_status', 2)->whereBetween('create_time', [$start, $end])->count();
    }
}
