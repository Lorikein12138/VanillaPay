<?php
use app\common\service\DeviceProvisionService;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;
use tests\Support\InMemoryDeviceRepository;

final class DeviceProvisionServiceTest extends TestCase
{
    public function testProvisionReusesExistingDeviceForMerchant(): void
    {
        $devices = new InMemoryDeviceRepository();
        $service = new DeviceProvisionService($devices, new FixedClock(1700000000));

        $first = $service->provision(10, '', 'https://pay.example.com');
        $second = $service->provision(10, '', 'https://pay.example.com');

        $this->assertSame($first['device_id'], $second['device_id']);
        $this->assertSame($first['device_key'], $second['device_key']);
        $this->assertCount(1, $devices->listByUser(10));
    }

    public function testDevicesMigrationEnforcesOneDevicePerMerchant(): void
    {
        $migration = file_get_contents(dirname(__DIR__, 2) . '/database/migrations/20260603100002_create_devices_table.php') ?: '';

        $this->assertStringContainsString("->addIndex(['user_id'], ['unique' => true])", $migration);
    }
}
