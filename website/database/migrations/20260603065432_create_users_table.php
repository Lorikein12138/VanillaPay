<?php

use think\migration\Migrator;

class CreateUsersTable extends Migrator
{
    /**
     * Change Method.
     *
     * Write your reversible migrations using this method.
     *
     * More information on writing migrations is available here:
     * http://docs.phinx.org/en/latest/migrations.html#the-abstractmigration-class
     *
     * The following commands can be used in this method and Phinx will
     * automatically reverse them when rolling back:
     *
     *    createTable
     *    renameTable
     *    addColumn
     *    renameColumn
     *    addIndex
     *    addForeignKey
     *
     * Remember to call "create()" or "update()" and NOT "save()" when working
     * with the Table class.
     */
    public function change()
    {
        $table = $this->table('users', [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '商户',
        ]);
        $table->addColumn('username', 'string', ['limit' => 50])
            ->addColumn('email', 'string', ['limit' => 120])
            ->addColumn('password_hash', 'string', ['limit' => 255])
            ->addColumn('pid', 'string', ['limit' => 32])
            ->addColumn('api_key', 'string', ['limit' => 64])
            ->addColumn('status', 'integer', ['limit' => 1, 'default' => 1, 'comment' => '0待审/1正常/2封禁'])
            ->addColumn('float_mode', 'enum', ['values' => ['up', 'down', 'both'], 'default' => 'up'])
            ->addColumn('float_step', 'decimal', ['precision' => 5, 'scale' => 2, 'default' => 0.01])
            ->addColumn('float_max', 'decimal', ['precision' => 5, 'scale' => 2, 'default' => 0.10])
            ->addColumn('order_timeout', 'integer', ['default' => 300])
            ->addColumn('login_fail_count', 'integer', ['default' => 0])
            ->addColumn('locked_until', 'datetime', ['null' => true])
            ->addColumn('last_login_at', 'datetime', ['null' => true])
            ->addColumn('last_login_ip', 'string', ['limit' => 45, 'null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addColumn('update_time', 'datetime', ['null' => true])
            ->addIndex(['username'], ['unique' => true])
            ->addIndex(['email'], ['unique' => true])
            ->addIndex(['pid'], ['unique' => true])
            ->create();
    }
}
