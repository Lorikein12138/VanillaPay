<?php
namespace tests\Support;

use app\common\repository\RiskEventRepositoryInterface;

final class InMemoryRiskEventRepository implements RiskEventRepositoryInterface
{
    /** @var array<int,array> */
    public array $rows = [];
    private int $auto = 0;

    public function create(array $data): int
    {
        $id = ++$this->auto;
        $this->rows[$id] = ['id' => $id] + $data;
        return $id;
    }

    public function paginate(array $filters, int $page, int $pageSize): array
    {
        return ['items' => array_values($this->rows), 'total' => count($this->rows), 'page' => $page, 'page_size' => $pageSize];
    }

    public function markHandled(int $id): void
    {
        $this->rows[$id]['handled'] = 1;
    }

    public function countByTypeBetween(string $type, string $start, string $end): int
    {
        return count(array_filter($this->rows, fn (array $row): bool => ($row['type'] ?? '') === $type
            && ($row['create_time'] ?? '') >= $start && ($row['create_time'] ?? '') <= $end));
    }
}
