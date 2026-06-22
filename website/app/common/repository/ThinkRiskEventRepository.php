<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkRiskEventRepository implements RiskEventRepositoryInterface
{
    private function table()
    {
        return Db::name('risk_events');
    }

    public function create(array $data): int
    {
        return (int) $this->table()->insertGetId($data + ['create_time' => date('Y-m-d H:i:s')]);
    }

    public function paginate(array $filters, int $page, int $pageSize): array
    {
        $query = $this->table();
        if (($filters['type'] ?? '') !== '') {
            $query->where('type', (string) $filters['type']);
        }
        $total = (int) (clone $query)->count();
        $items = $query->order('id', 'desc')->page($page, $pageSize)->select()->toArray();
        return ['items' => $items, 'total' => $total, 'page' => $page, 'page_size' => $pageSize];
    }

    public function markHandled(int $id): void
    {
        $this->table()->where('id', $id)->update(['handled' => 1]);
    }

    public function countByTypeBetween(string $type, string $start, string $end): int
    {
        return (int) $this->table()->where('type', $type)->whereBetween('create_time', [$start, $end])->count();
    }
}
