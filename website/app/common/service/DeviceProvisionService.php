<?php
namespace app\common\service;

use app\common\repository\DeviceRepositoryInterface;
use app\common\support\Clock;

final class DeviceProvisionService
{
    public function __construct(private DeviceRepositoryInterface $devices, private Clock $clock)
    {
    }

    public function provision(int $userId, string $name, string $serverUrl): array
    {
        $key = bin2hex(random_bytes(16));
        $id = $this->devices->create([
            'user_id' => $userId,
            'device_key' => $key,
            'device_name' => $name !== '' ? $name : '未命名设备',
            'status' => 'offline',
            'create_time' => $this->clock->now(),
        ]);

        return [
            'device_id' => $id,
            'device_key' => $key,
            'binding_payload' => rtrim($serverUrl, '/') . '|' . $id . '|' . $key,
        ];
    }
}
