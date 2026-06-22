<?php
namespace app\command;

use app\common\repository\DeviceRepositoryInterface;
use app\common\repository\RiskEventRepositoryInterface;
use app\common\support\Clock;
use think\console\Command;
use think\console\Input;
use think\console\Output;

class DeviceCheck extends Command
{
    protected function configure(): void
    {
        $this->setName('vanilla:device-check')->setDescription('Mark stale devices offline and raise risk events');
    }

    protected function execute(Input $input, Output $output): int
    {
        $clock = app(Clock::class);
        $threshold = date('Y-m-d H:i:s', $clock->timestamp() - 90);
        $devices = app(DeviceRepositoryInterface::class);
        $risks = app(RiskEventRepositoryInterface::class);
        $count = 0;
        foreach ($devices->listOnlineStale($threshold) as $device) {
            $devices->markOffline((int) $device['id']);
            $risks->create([
                'type' => 'device_offline',
                'user_id' => (int) $device['user_id'],
                'device_id' => (int) $device['id'],
                'level' => 'warning',
                'detail' => '',
            ]);
            $count++;
        }
        $output->writeln("offline devices: {$count}");
        return 0;
    }
}
