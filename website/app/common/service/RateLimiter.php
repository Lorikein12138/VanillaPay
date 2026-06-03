<?php
namespace app\common\service;

use app\common\support\CacheStore;

final class RateLimiter
{
    public function __construct(private CacheStore $cache)
    {
    }

    public function allow(string $key, int $maxHits, int $windowSeconds): bool
    {
        return $this->cache->increment('rl:' . $key, $windowSeconds) <= $maxHits;
    }
}
