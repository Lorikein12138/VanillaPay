<?php

use think\migration\Migrator;

class CreateOrdersTable extends Migrator
{
    public function change()
    {
        $this->table('orders', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '订单',
        ])->addColumn('order_no', 'string', ['limit' => 32])
            ->addColumn('out_trade_no', 'string', ['limit' => 64])
            ->addColumn('user_id', 'integer')
            ->addColumn('protocol', 'enum', ['values' => ['epay', 'codepay', 'yuanpay']])
            ->addColumn('channel', 'enum', ['values' => ['wxpay', 'alipay']])
            ->addColumn('product_name', 'string', ['limit' => 120, 'default' => ''])
            ->addColumn('money', 'decimal', ['precision' => 10, 'scale' => 2])
            ->addColumn('real_amount', 'decimal', ['precision' => 10, 'scale' => 2])
            ->addColumn('qrcode_id', 'integer', ['default' => 0])
            ->addColumn('status', 'enum', ['values' => ['pending', 'paid', 'expired', 'closed'], 'default' => 'pending'])
            ->addColumn('notify_url', 'string', ['limit' => 500, 'default' => ''])
            ->addColumn('return_url', 'string', ['limit' => 500, 'default' => ''])
            ->addColumn('param', 'string', ['limit' => 255, 'default' => ''])
            ->addColumn('client_ip', 'string', ['limit' => 45, 'default' => ''])
            ->addColumn('device_id', 'integer', ['null' => true])
            ->addColumn('device_trade_no', 'string', ['limit' => 64, 'default' => ''])
            ->addColumn('paid_at', 'datetime', ['null' => true])
            ->addColumn('expire_at', 'datetime', ['null' => true])
            ->addColumn('notify_status', 'integer', ['limit' => 1, 'default' => 0, 'comment' => '0未通知/1成功/2失败'])
            ->addColumn('notify_count', 'integer', ['default' => 0])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['user_id', 'out_trade_no'], ['unique' => true])
            ->addIndex(['user_id', 'channel', 'real_amount', 'status'])
            ->addIndex(['status', 'expire_at'])
            ->addIndex(['order_no'], ['unique' => true])
            ->create();
    }
}
