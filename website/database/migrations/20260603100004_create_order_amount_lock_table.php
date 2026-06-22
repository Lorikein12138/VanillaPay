<?php

use think\migration\Migrator;

class CreateOrderAmountLockTable extends Migrator
{
    public function change()
    {
        $this->table('order_amount_lock', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '金额并发锁',
        ])->addColumn('user_id', 'integer')
            ->addColumn('channel', 'enum', ['values' => ['wxpay', 'alipay']])
            ->addColumn('real_amount', 'decimal', ['precision' => 10, 'scale' => 2])
            ->addColumn('order_id', 'integer', ['default' => 0])
            ->addColumn('expire_at', 'datetime', ['null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['user_id', 'channel', 'real_amount'], ['unique' => true])
            ->create();
    }
}
