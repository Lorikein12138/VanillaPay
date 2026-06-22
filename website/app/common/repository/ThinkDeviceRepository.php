<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkDeviceRepository implements DeviceRepositoryInterface
{
    private function table()
    {
        return Db::name('devices');
    }

    public function create(array $data): int
    {
        return (int) $this->table()->insertGetId($data);
    }

    public function findById(int $id): ?array
    {
        return $this->table()->where('id', $id)->find();
    }

    public function touchHeartbeat(int $id, array $data): void
    {
        $this->table()->where('id', $id)->update($data);
    }

    public function listOnlineStale(string $threshold): array
    {
        return $this->table()->where('status', 'online')->where('last_heartbeat', '<', $threshold)->select()->toArray();
    }

    public function markOffline(int $id): void
    {
        $this->table()->where('id', $id)->update(['status' => 'offline']);
    }

    public function listByUser(int $userId): array
    {
        return $this->table()->where('user_id', $userId)->order('id', 'desc')->select()->toArray();
    }

    public function deleteForUser(int $id, int $userId): void
    {
        $this->table()->where('id', $id)->where('user_id', $userId)->delete();
    }
}
