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
        return $this->cache->putIfAbsent('replay:' . $key, '1', $ttlSeconds);
    }
}
