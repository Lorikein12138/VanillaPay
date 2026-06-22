<?php
namespace app\common\repository;

interface DeviceRepositoryInterface
{
    public function create(array $data): int;
    public function findById(int $id): ?array;
    public function touchHeartbeat(int $id, array $data): void;
    public function listOnlineStale(string $threshold): array;
    public function markOffline(int $id): void;
    public function listByUser(int $userId): array;
    public function deleteForUser(int $id, int $userId): void;
}
