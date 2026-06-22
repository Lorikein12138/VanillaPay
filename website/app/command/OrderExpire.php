<?php
namespace app\command;

use app\common\service\OrderExpirationService;
use think\console\Command;
use think\console\Input;
use think\console\Output;

class OrderExpire extends Command
{
    protected function configure(): void
    {
        $this->setName('vanilla:order-expire')->setDescription('Expire timed-out pending orders and release amount locks');
    }

    protected function execute(Input $input, Output $output): int
    {
        $result = app(OrderExpirationService::class)->refresh();
        $output->writeln("expired orders: {$result['orders']}, released locks: {$result['locks']}");
        return 0;
    }
}
