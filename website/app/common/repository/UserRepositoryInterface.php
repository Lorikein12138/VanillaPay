<?php
namespace app\common\repository;

interface UserRepositoryInterface
{
    public function findByUsername(string $username): ?array;
    public function findByEmail(string $email): ?array;
    public function findById(int $id): ?array;
    public function findByPid(string $pid): ?array;
    public function existsUsername(string $username): bool;
    public function existsEmail(string $email): bool;
    public function create(array $data): int;
    public function update(int $id, array $data): void;
    public function paginate(array $filters, int $page, int $pageSize): array;
    public function setStatus(int $id, int $status): void;
    public function resetApiKey(int $id, string $apiKey): void;
    public function updateFloat(int $id, array $data): void;
}
