<?php

use think\migration\Migrator;
use think\facade\Db;

class AddUniqueUserIdToDevicesTable extends Migrator
{
    public function up()
    {
        $duplicates = Db::name('devices')
            ->field('user_id, MAX(id) AS keep_id, COUNT(*) AS total')
            ->group('user_id')
            ->having('COUNT(*) > 1')
            ->select()
            ->toArray();
        foreach ($duplicates as $row) {
            Db::name('devices')
                ->where('user_id', (int) $row['user_id'])
                ->where('id', '<>', (int) $row['keep_id'])
                ->delete();
        }

        $table = $this->table('devices');
        if ($table->hasIndex(['user_id'])) {
            $table->removeIndex(['user_id'])->update();
        }
        $table->addIndex(['user_id'], ['unique' => true])->update();
    }

    public function down()
    {
        $table = $this->table('devices');
        if ($table->hasIndex(['user_id'])) {
            $table->removeIndex(['user_id'])->update();
        }
        $table->addIndex(['user_id'])->update();
    }
}
