<?php
namespace app\common\repository;

interface RiskEventRepositoryInterface
{
    public function create(array $data): int;
    public function paginate(array $filters, int $page, int $pageSize): array;
    public function markHandled(int $id): void;
    public function countByTypeBetween(string $type, string $start, string $end): int;
}
