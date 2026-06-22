<?php
namespace tests\Support;

use app\common\repository\AmountLockRepositoryInterface;

final class InMemoryAmountLockRepository implements AmountLockRepositoryInterface
{
    /** @var array<string,array> */
    public array $locks = [];

    private function key(int $userId, string $channel, int $amountCents): string
    {
        return $userId . '|' . $channel . '|' . $amountCents;
    }

    public function tryAcquire(int $userId, string $channel, int $amountCents, string $expireAt): bool
    {
        $key = $this->key($userId, $channel, $amountCents);
        if (isset($this->locks[$key])) {
            return false;
        }

        $this->locks[$key] = [
            'user_id' => $userId,
            'channel' => $channel,
            'amount_cents' => $amountCents,
            'order_id' => 0,
            'expire_at' => $expireAt,
        ];
        return true;
    }

    public function attachOrder(int $userId, string $channel, int $amountCents, int $orderId): void
    {
        $this->locks[$this->key($userId, $channel, $amountCents)]['order_id'] = $orderId;
    }

    public function release(int $userId, string $channel, int $amountCents): void
    {
        unset($this->locks[$this->key($userId, $channel, $amountCents)]);
    }

    public function releaseExpired(string $now): int
    {
        $count = 0;
        foreach ($this->locks as $key => $lock) {
            if (($lock['expire_at'] ?? '') <= $now) {
                unset($this->locks[$key]);
                $count++;
            }
        }
        return $count;
    }
}
