<?php
namespace app\common\repository;

interface AdminRepositoryInterface
{
    public function findByUsername(string $username): ?array;
    public function findById(int $id): ?array;
    public function create(array $data): int;
    public function update(int $id, array $data): void;
}
