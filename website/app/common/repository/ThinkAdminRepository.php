<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkAdminRepository implements AdminRepositoryInterface
{
    private function table()
    {
        return Db::name('admins');
    }

    public function findByUsername(string $username): ?array
    {
        return $this->table()->where('username', $username)->find();
    }

    public function findById(int $id): ?array
    {
        return $this->table()->where('id', $id)->find();
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
