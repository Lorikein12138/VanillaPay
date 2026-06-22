<?php
namespace app\common\repository;

interface SettingsRepositoryInterface
{
    public function get(string $key, ?string $default = null): ?string;
    public function set(string $key, string $value): void;
    public function all(): array;
}
