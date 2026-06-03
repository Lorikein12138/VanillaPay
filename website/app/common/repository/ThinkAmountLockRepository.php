<?php
namespace app\common\repository;

use app\common\support\Money;
use think\facade\Db;

class ThinkAmountLockRepository implements AmountLockRepositoryInterface
{
    private function table()
    {
        return Db::name('order_amount_lock');
    }

    public function tryAcquire(int $userId, string $channel, int $amountCents, string $expireAt): bool
    {
        try {
            $this->table()->insert([
                'user_id' => $userId,
                'channel' => $channel,
                'real_amount' => Money::fromCents($amountCents),
                'order_id' => 0,
                'expire_at' => $expireAt,
                'create_time' => date('Y-m-d H:i:s'),
            ]);
            return true;
        } catch (\Throwable) {
            return false;
        }
    }

    public function attachOrder(int $userId, string $channel, int $amountCents, int $orderId): void
    {
        $this->table()->where('user_id', $userId)->where('channel', $channel)->where('real_amount', Money::fromCents($amountCents))->update(['order_id' => $orderId]);
    }

    public function release(int $userId, string $channel, int $amountCents): void
    {
        $this->table()->where('user_id', $userId)->where('channel', $channel)->where('real_amount', Money::fromCents($amountCents))->delete();
    }

    public function releaseExpired(string $now): int
    {
        return (int) $this->table()->where('expire_at', '<=', $now)->delete();
    }
}
