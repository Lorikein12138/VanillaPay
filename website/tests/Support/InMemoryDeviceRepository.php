<?php
namespace tests\Support;

use app\common\repository\DeviceRepositoryInterface;

final class InMemoryDeviceRepository implements DeviceRepositoryInterface
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

    public function findById(int $id): ?array
    {
        return $this->rows[$id] ?? null;
    }

    public function touchHeartbeat(int $id, array $data): void
    {
        $this->rows[$id] = array_merge($this->rows[$id] ?? ['id' => $id], $data);
    }

    public function listOnlineStale(string $threshold): array
    {
        return array_values(array_filter($this->rows, fn (array $row): bool => ($row['status'] ?? '') === 'online'
            && !empty($row['last_heartbeat']) && $row['last_heartbeat'] < $threshold));
    }

    public function markOffline(int $id): void
    {
        $this->rows[$id]['status'] = 'offline';
    }

    public function listByUser(int $userId): array
    {
        return array_values(array_filter($this->rows, fn (array $row): bool => (int) $row['user_id'] === $userId));
    }

    public function deleteForUser(int $id, int $userId): void
    {
        if ((int) ($this->rows[$id]['user_id'] ?? 0) === $userId) {
            unset($this->rows[$id]);
        }
    }
}
