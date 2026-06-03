<?php
namespace tests\Support;

use app\common\repository\UserRepositoryInterface;

final class InMemoryUserRepository implements UserRepositoryInterface
{
    /** @var array<int,array> */
    public array $rows = [];
    private int $auto = 0;

    public function findByUsername(string $username): ?array
    {
        foreach ($this->rows as $row) {
            if ($row['username'] === $username) {
                return $row;
            }
        }
        return null;
    }

    public function findByEmail(string $email): ?array
    {
        foreach ($this->rows as $row) {
            if ($row['email'] === $email) {
                return $row;
            }
        }
        return null;
    }

    public function findById(int $id): ?array
    {
        return $this->rows[$id] ?? null;
    }

    public function existsUsername(string $username): bool
    {
        return $this->findByUsername($username) !== null;
    }

    public function existsEmail(string $email): bool
    {
        return $this->findByEmail($email) !== null;
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
