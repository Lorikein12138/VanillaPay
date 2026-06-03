<?php
namespace tests\Support;

use app\common\repository\AdminRepositoryInterface;

final class InMemoryAdminRepository implements AdminRepositoryInterface
{
    /** @var array<int,array> */
    public array $rows = [];
    private int $auto = 0;

    public function findByUsername(string $username): ?array
    {
        foreach ($this->rows as $row) {
            if (($row['username'] ?? '') === $username) {
                return $row;
            }
        }
        return null;
    }

    public function findById(int $id): ?array
    {
        return $this->rows[$id] ?? null;
    }

    public function create(array $data): int
    {
        $id = ++$this->auto;
        $this->rows[$id] = ['id' => $id] + $data;
        return $id;
    }

    public function update(int $id, array $data): void
    {
        $this->rows[$id] = array_merge($this->rows[$id] ?? ['id' => $id], $data);
    }
}
