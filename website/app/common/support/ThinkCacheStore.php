<?php
namespace app\common\support;

use think\facade\Cache;

final class ThinkCacheStore implements CacheStore
{
    public function increment(string $key, int $ttlSeconds): int
    {
        if (!Cache::has($key)) {
            Cache::set($key, 0, $ttlSeconds);
        }
        return (int) Cache::inc($key);
    }

    public function has(string $key): bool
    {
        return Cache::has($key);
    }

    public function put(string $key, string $value, int $ttlSeconds): void
    {
        Cache::set($key, $value, $ttlSeconds);
    }
}
