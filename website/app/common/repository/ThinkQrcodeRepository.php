<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkQrcodeRepository implements QrcodeRepositoryInterface
{
    private function table()
    {
        return Db::name('merchant_qrcodes');
    }

    public function create(array $data): int
    {
        return (int) $this->table()->insertGetId($data);
    }

    public function findById(int $id): ?array
    {
        return $this->table()->where('id', $id)->find();
    }

    public function findEnabledByUserChannel(int $userId, string $channel): ?array
    {
        return $this->table()->where('user_id', $userId)->where('channel', $channel)->where('status', 1)->order('id', 'desc')->find();
    }

    public function listByUser(int $userId): array
    {
        $rows = $this->table()
            ->where('user_id', $userId)
            ->whereIn('channel', ['wxpay', 'alipay'])
            ->order('id', 'desc')
            ->select()
            ->toArray();
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
        $this->table()->where('id', $id)->where('user_id', $userId)->delete();
    }

    public function deleteForUserChannel(int $userId, string $channel): void
    {
        $this->table()->where('user_id', $userId)->where('channel', $channel)->delete();
    }
}
