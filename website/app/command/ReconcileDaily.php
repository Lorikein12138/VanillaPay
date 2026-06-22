<?php
namespace app\command;

use app\common\service\ReconciliationService;
use think\console\Command;
use think\console\Input;
use think\console\Output;

class ReconcileDaily extends Command
{
    protected function configure(): void
    {
        $this->setName('vanilla:reconcile-daily')->setDescription('Print yesterday reconciliation summary');
    }

    protected function execute(Input $input, Output $output): int
    {
        $output->writeln(json_encode(app(ReconciliationService::class)->daily(date('Y-m-d', strtotime('-1 day'))), JSON_UNESCAPED_UNICODE));
        return 0;
    }
}
