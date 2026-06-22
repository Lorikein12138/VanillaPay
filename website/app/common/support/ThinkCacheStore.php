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

    public function putIfAbsent(string $key, string $value, int $ttlSeconds): bool
    {
        $lock = $this->lockHandle($key);
        if (!flock($lock, LOCK_EX)) {
            fclose($lock);
            return false;
        }

        try {
            if (Cache::has($key)) {
                return false;
            }

            return Cache::set($key, $value, $ttlSeconds);
        } finally {
            flock($lock, LOCK_UN);
            fclose($lock);
        }
    }

    /**
     * @return resource
     */
    private function lockHandle(string $key)
    {
        $dir = app()->getRuntimePath() . 'locks';
        if (!is_dir($dir)) {
            mkdir($dir, 0755, true);
        }

        $handle = fopen($dir . DIRECTORY_SEPARATOR . hash('sha256', $key) . '.lock', 'c');
        if ($handle === false) {
            throw new \RuntimeException('Unable to open cache claim lock');
        }

        return $handle;
    }
}
