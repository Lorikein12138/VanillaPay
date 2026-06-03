<?php
namespace app\common\service;

use app\common\support\CacheStore;

final class ReplayGuard
{
    public function __construct(private CacheStore $cache)
    {
    }

    public function firstUse(string $key, int $ttlSeconds): bool
    {
        $cacheKey = 'replay:' . $key;
        if ($this->cache->has($cacheKey)) {
            return false;
        }
        $this->cache->put($cacheKey, '1', $ttlSeconds);
        return true;
    }
}
