<?php
namespace app\command;

use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Clock;
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
        $now = app(Clock::class)->now();
        $orders = app(OrderRepositoryInterface::class)->markExpiredBatch($now);
        $locks = app(AmountLockRepositoryInterface::class)->releaseExpired($now);
        $output->writeln("expired orders: {$orders}, released locks: {$locks}");
        return 0;
    }
}
