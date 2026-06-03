<?php
namespace app\common\support;

interface CacheStore
{
    public function increment(string $key, int $ttlSeconds): int;
    public function has(string $key): bool;
    public function put(string $key, string $value, int $ttlSeconds): void;
}
