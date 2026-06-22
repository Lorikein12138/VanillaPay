<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkOperationLogRepository implements OperationLogRepositoryInterface
{
    public function record(array $data): void
    {
        Db::name('operation_logs')->insert($data + ['create_time' => date('Y-m-d H:i:s')]);
    }
}
