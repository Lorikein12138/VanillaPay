<?php

use think\migration\Migrator;

class CreateLoginLogsTable extends Migrator
{
    public function change()
    {
        $this->table('login_logs', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '登录日志',
        ])->addColumn('user_type', 'string', ['limit' => 10])
            ->addColumn('uid', 'integer', ['default' => 0])
            ->addColumn('ip', 'string', ['limit' => 45, 'default' => ''])
            ->addColumn('ua', 'string', ['limit' => 255, 'default' => ''])
            ->addColumn('result', 'string', ['limit' => 10, 'default' => ''])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['user_type', 'uid'])
            ->create();
    }
}
