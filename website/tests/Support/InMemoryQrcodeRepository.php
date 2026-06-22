<?php
namespace tests\Support;

use app\common\repository\QrcodeRepositoryInterface;

final class InMemoryQrcodeRepository implements QrcodeRepositoryInterface
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

    public function findEnabledByUserChannel(int $userId, string $channel): ?array
    {
        foreach ($this->rows as $row) {
            if ((int) $row['user_id'] === $userId && ($row['channel'] ?? '') === $channel && (int) ($row['status'] ?? 1) === 1) {
                return $row;
            }
        }
        return null;
    }

    public function listByUser(int $userId): array
    {
        $rows = array_values(array_filter(
            $this->rows,
            fn (array $row): bool => (int) $row['user_id'] === $userId && in_array((string) ($row['channel'] ?? ''), ['wxpay', 'alipay'], true)
        ));
        usort($rows, fn (array $a, array $b): int => (int) $b['id'] <=> (int) $a['id']);

        $latestByChannel = [];
        foreach ($rows as $row) {
            $channel = (string) ($row['channel'] ?? '');
            if ($channel !== '' && !isset($latestByChannel[$channel])) {
                $latestByChannel[$channel] = $row;
            }
        }

        return array_values($latestByChannel);
    }

    public function deleteForUser(int $id, int $userId): void
    {
        if ((int) ($this->rows[$id]['user_id'] ?? 0) === $userId) {
            unset($this->rows[$id]);
        }
    }

    public function deleteForUserChannel(int $userId, string $channel): void
    {
        foreach ($this->rows as $id => $row) {
            if ((int) ($row['user_id'] ?? 0) === $userId && ($row['channel'] ?? '') === $channel) {
                unset($this->rows[$id]);
            }
        }
    }
}
