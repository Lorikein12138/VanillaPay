<?php
namespace tests\Support;

use app\common\support\CacheStore;

final class ArrayCacheStore implements CacheStore
{
    public array $data = [];

    public function increment(string $key, int $ttlSeconds): int
    {
        return $this->data[$key] = (int) ($this->data[$key] ?? 0) + 1;
    }

    public function has(string $key): bool
    {
        return array_key_exists($key, $this->data);
    }

    public function put(string $key, string $value, int $ttlSeconds): void
    {
        $this->data[$key] = $value;
    }

    public function putIfAbsent(string $key, string $value, int $ttlSeconds): bool
    {
        if ($this->has($key)) {
            return false;
        }

        $this->put($key, $value, $ttlSeconds);
        return true;
    }
}
