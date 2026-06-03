<?php

use think\migration\Migrator;

class CreateSettingsTable extends Migrator
{
    public function change()
    {
        $this->table('settings', [
            'id' => false,
            'primary_key' => 'skey',
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '系统设置',
        ])->addColumn('skey', 'string', ['limit' => 64, 'null' => false])
            ->addColumn('svalue', 'text', ['null' => true])
            ->create();
    }
}
