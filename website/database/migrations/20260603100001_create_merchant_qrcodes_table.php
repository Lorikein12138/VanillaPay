<?php

use think\migration\Migrator;

class CreateMerchantQrcodesTable extends Migrator
{
    public function change()
    {
        $this->table('merchant_qrcodes', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '收款码',
        ])->addColumn('user_id', 'integer')
            ->addColumn('channel', 'enum', ['values' => ['wxpay', 'alipay']])
            ->addColumn('name', 'string', ['limit' => 50, 'default' => ''])
            ->addColumn('qr_image_path', 'string', ['limit' => 255, 'default' => ''])
            ->addColumn('qr_content', 'string', ['limit' => 255, 'default' => ''])
            ->addColumn('status', 'integer', ['limit' => 1, 'default' => 1])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addIndex(['user_id', 'channel', 'status'])
            ->create();
    }
}
