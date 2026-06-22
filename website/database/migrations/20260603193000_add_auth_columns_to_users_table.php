<?php

use think\migration\Migrator;

class AddAuthColumnsToUsersTable extends Migrator
{
    public function up()
    {
        if (!$this->hasTable('users')) {
            return;
        }

        $table = $this->table('users');

        if (!$table->hasColumn('float_mode')) {
            $table->addColumn('float_mode', 'enum', ['values' => ['up', 'down', 'both'], 'default' => 'up']);
        }
        if (!$table->hasColumn('float_step')) {
            $table->addColumn('float_step', 'decimal', ['precision' => 5, 'scale' => 2, 'default' => 0.01]);
        }
        if (!$table->hasColumn('float_max')) {
            $table->addColumn('float_max', 'decimal', ['precision' => 5, 'scale' => 2, 'default' => 0.10]);
        }
        if (!$table->hasColumn('order_timeout')) {
            $table->addColumn('order_timeout', 'integer', ['default' => 300]);
        }
        if (!$table->hasColumn('login_fail_count')) {
            $table->addColumn('login_fail_count', 'integer', ['default' => 0]);
        }
        if (!$table->hasColumn('locked_until')) {
            $table->addColumn('locked_until', 'datetime', ['null' => true]);
        }
        if (!$table->hasColumn('last_login_at')) {
            $table->addColumn('last_login_at', 'datetime', ['null' => true]);
        }
        if (!$table->hasColumn('last_login_ip')) {
            $table->addColumn('last_login_ip', 'string', ['limit' => 45, 'null' => true]);
        }

        $table->update();
    }

    public function down()
    {
        if (!$this->hasTable('users')) {
            return;
        }

        $table = $this->table('users');

        foreach (['last_login_ip', 'last_login_at', 'locked_until', 'login_fail_count', 'order_timeout', 'float_max', 'float_step', 'float_mode'] as $column) {
            if ($table->hasColumn($column)) {
                $table->removeColumn($column);
            }
        }

        $table->update();
    }
}
