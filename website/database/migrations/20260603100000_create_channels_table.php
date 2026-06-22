<?php

use think\migration\Migrator;

class CreateChannelsTable extends Migrator
{
    public function change()
    {
        $table = $this->table('channels', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '渠道',
        ]);
        $table->addColumn('code', 'string', ['limit' => 20])
            ->addColumn('name', 'string', ['limit' => 30])
            ->addColumn('enabled', 'boolean', ['default' => true])
            ->addIndex(['code'], ['unique' => true])
            ->create();

        $this->table('channels')->insert([
            ['code' => 'wxpay', 'name' => '微信', 'enabled' => 1],
            ['code' => 'alipay', 'name' => '支付宝', 'enabled' => 1],
        ])->saveData();
    }
}
