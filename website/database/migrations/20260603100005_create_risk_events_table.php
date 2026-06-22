<?php

use think\migration\Migrator;

class CreateRiskEventsTable extends Migrator
{
    public function change()
    {
        $this->table('risk_events', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '风控事件',
        ])->addColumn('type', 'string', ['limit' => 40])
            ->addColumn('user_id', 'integer', ['default' => 0])
            ->addColumn('device_id', 'integer', ['default' => 0])
            ->addColumn('order_id', 'integer', ['default' => 0])
            ->addColumn('level', 'string', ['limit' => 10, 'default' => 'info'])
            ->addColumn('detail', 'text', ['null' => true])
            ->addColumn('handled', 'boolean', ['default' => false])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['type'])
            ->addIndex(['user_id'])
            ->create();
    }
}
