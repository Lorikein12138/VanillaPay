<?php

use think\migration\Migrator;

class CreateOperationLogsTable extends Migrator
{
    public function change()
    {
        $this->table('operation_logs', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '操作日志',
        ])->addColumn('actor_type', 'string', ['limit' => 10])
            ->addColumn('actor_id', 'integer', ['default' => 0])
            ->addColumn('action', 'string', ['limit' => 60])
            ->addColumn('target', 'string', ['limit' => 120, 'default' => ''])
            ->addColumn('ip', 'string', ['limit' => 45, 'default' => ''])
            ->addColumn('detail', 'text', ['null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['actor_type', 'actor_id'])
            ->addIndex(['action'])
            ->create();
    }
}
