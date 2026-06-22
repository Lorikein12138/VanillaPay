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

    public function findByPid(string $pid): ?array
    {
        return $this->table()->where('pid', $pid)->find();
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

    public function paginate(array $filters, int $page, int $pageSize): array
    {
        $query = $this->table();
        $keyword = trim((string) ($filters['keyword'] ?? ''));
        if ($keyword !== '') {
            $query->whereLike('username|email|pid', '%' . $keyword . '%');
        }
        if (($filters['status'] ?? '') !== '') {
            $query->where('status', (int) $filters['status']);
        }

        $total = (int) (clone $query)->count();
        $items = $query->order('id', 'desc')
            ->page($page, $pageSize)
            ->select()
            ->toArray();

        return ['items' => $items, 'total' => $total, 'page' => $page, 'page_size' => $pageSize];
    }

    public function setStatus(int $id, int $status): void
    {
        $this->update($id, ['status' => $status, 'update_time' => date('Y-m-d H:i:s')]);
    }

    public function resetApiKey(int $id, string $apiKey): void
    {
        $this->update($id, ['api_key' => $apiKey, 'update_time' => date('Y-m-d H:i:s')]);
    }

    public function updateFloat(int $id, array $data): void
    {
        $this->update($id, $data + ['update_time' => date('Y-m-d H:i:s')]);
    }
}
