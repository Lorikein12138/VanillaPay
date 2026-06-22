<?php
namespace app\command;

use app\common\repository\CallbackLogRepositoryInterface;
use app\common\service\CallbackSender;
use app\common\support\Clock;
use think\console\Command;
use think\console\Input;
use think\console\Output;

class CallbackRetry extends Command
{
    private const MAX_ATTEMPTS = 7;

    protected function configure(): void
    {
        $this->setName('vanilla:callback-retry')->setDescription('Retry failed downstream callbacks with backoff');
    }

    protected function execute(Input $input, Output $output): int
    {
        $logs = app(CallbackLogRepositoryInterface::class)->findRetryable(app(Clock::class)->now(), self::MAX_ATTEMPTS);
        $sender = app(CallbackSender::class);
        $ok = 0;
        foreach ($logs as $log) {
            if ($sender->sendForOrder((int) $log['order_id'])) {
                $ok++;
            }
        }
        $output->writeln('retried: ' . count($logs) . ', succeeded: ' . $ok);
        return 0;
    }
}
