<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkLoginLogRepository implements LoginLogRepositoryInterface
{
    public function record(array $data): void
    {
        Db::name('login_logs')->insert($data + ['create_time' => date('Y-m-d H:i:s')]);
    }
}
