<?php
namespace app\common\repository;

interface QrcodeRepositoryInterface
{
    public function create(array $data): int;
    public function findById(int $id): ?array;
    public function findEnabledByUserChannel(int $userId, string $channel): ?array;
    public function listByUser(int $userId): array;
    public function deleteForUser(int $id, int $userId): void;
}
