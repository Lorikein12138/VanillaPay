<?php

use think\migration\Migrator;
use think\migration\db\Column;

class CreateAdminsTable extends Migrator
{
    private function tableName(string $name): string
    {
        return env('DB_PREFIX', 'vp_') . $name;
    }

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
        $table = $this->table($this->tableName('admins'), [
            'engine' => 'InnoDB',
            'collation' => 'utf8mb4_general_ci',
            'comment' => '超管',
        ]);
        $table->addColumn('username', 'string', ['limit' => 50])
            ->addColumn('password_hash', 'string', ['limit' => 255])
            ->addColumn('status', 'integer', ['limit' => 1, 'default' => 1])
            ->addColumn('last_login_at', 'datetime', ['null' => true])
            ->addColumn('last_login_ip', 'string', ['limit' => 45, 'null' => true])
            ->addColumn('create_time', 'datetime', ['null' => true])
            ->addColumn('update_time', 'datetime', ['null' => true])
            ->addIndex(['username'], ['unique' => true])
            ->create();
    }
}
