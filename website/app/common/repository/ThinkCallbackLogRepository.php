<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkCallbackLogRepository implements CallbackLogRepositoryInterface
{
    private function table()
    {
        return Db::name('callback_logs');
    }

    public function upsertForOrder(int $orderId, array $data): void
    {
        $data['order_id'] = $orderId;
        $data['update_time'] = $data['update_time'] ?? date('Y-m-d H:i:s');
        if ($this->findByOrder($orderId)) {
            $this->table()->where('order_id', $orderId)->update($data);
            return;
        }
        $this->table()->insert($data + ['create_time' => date('Y-m-d H:i:s')]);
    }

    public function findByOrder(int $orderId): ?array
    {
        return $this->table()->where('order_id', $orderId)->find();
    }

    public function findRetryable(string $now, int $maxAttempts): array
    {
        return $this->table()
            ->where('success', 0)
            ->where('attempts', '<', $maxAttempts)
            ->where('next_retry_at', '<=', $now)
            ->select()
            ->toArray();
    }
}
