<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkUserRepository implements UserRepositoryInterface
{
    private function table()
    {
        return Db::name('users');
    }

    public function findByUsername(string $username): ?array
    {
        return $this->table()->where('username', $username)->find();
    }

    public function findByEmail(string $email): ?array
    {
        return $this->table()->where('email', $email)->find();
    }

    public function findById(int $id): ?array
    {
        return $this->table()->where('id', $id)->find();
    }

    public function existsUsername(string $username): bool
    {
        return $this->table()->where('username', $username)->count() > 0;
    }

    public function existsEmail(string $email): bool
    {
        return $this->table()->where('email', $email)->count() > 0;
    }

    public function create(array $data): int
    {
        return (int) $this->table()->insertGetId($data);
    }

    public function update(int $id, array $data): void
    {
        $this->table()->where('id', $id)->update($data);
    }
}
