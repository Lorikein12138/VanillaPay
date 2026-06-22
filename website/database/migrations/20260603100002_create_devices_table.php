<?php

use think\migration\Migrator;

class CreateDevicesTable extends Migrator
{
    public function change()
    {
        $this->table('devices', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '安卓设备',
        ])->addColumn('user_id', 'integer')
            ->addColumn('device_key', 'string', ['limit' => 64])
            ->addColumn('device_name', 'string', ['limit' => 50, 'default' => ''])
            ->addColumn('status', 'enum', ['values' => ['online', 'offline'], 'default' => 'offline'])
            ->addColumn('last_heartbeat', 'datetime', ['null' => true])
            ->addColumn('last_ip', 'string', ['limit' => 45, 'null' => true])
            ->addColumn('app_version', 'string', ['limit' => 20, 'null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['user_id'], ['unique' => true])
            ->create();
    }
}
