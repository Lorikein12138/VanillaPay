<?php
namespace app\common\repository;

use think\facade\Db;

class ThinkSettingsRepository implements SettingsRepositoryInterface
{
    private function table()
    {
        return Db::name('settings');
    }

    public function get(string $key, ?string $default = null): ?string
    {
        $row = $this->table()->where('skey', $key)->find();
        return $row ? (string) $row['svalue'] : $default;
    }

    public function set(string $key, string $value): void
    {
        if ($this->table()->where('skey', $key)->find()) {
            $this->table()->where('skey', $key)->update(['svalue' => $value]);
            return;
        }
        $this->table()->insert(['skey' => $key, 'svalue' => $value]);
    }

    public function all(): array
    {
        $out = [];
        foreach ($this->table()->select()->toArray() as $row) {
            $out[$row['skey']] = $row['svalue'];
        }
        return $out;
    }
}
