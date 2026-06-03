<?php

use think\migration\Migrator;

class CreateCallbackLogsTable extends Migrator
{
    public function change()
    {
        $this->table('callback_logs', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '回调日志',
        ])->addColumn('order_id', 'integer')
            ->addColumn('url', 'string', ['limit' => 500, 'default' => ''])
            ->addColumn('request_body', 'text', ['null' => true])
            ->addColumn('response_body', 'text', ['null' => true])
            ->addColumn('http_code', 'integer', ['default' => 0])
            ->addColumn('success', 'boolean', ['default' => false])
            ->addColumn('attempts', 'integer', ['default' => 0])
            ->addColumn('next_retry_at', 'datetime', ['null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addColumn('update_time', 'datetime', ['null' => true])
            ->addIndex(['order_id'], ['unique' => true])
            ->addIndex(['success', 'next_retry_at'])
            ->create();
    }
}
