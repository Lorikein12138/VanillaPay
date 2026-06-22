<?php
namespace app\command;

use app\common\repository\AdminRepositoryInterface;
use app\common\service\PasswordHasher;
use think\console\Command;
use think\console\Input;
use think\console\input\Argument;
use think\console\Output;

class AdminCreate extends Command
{
    protected function configure(): void
    {
        $this->setName('vanilla:admin-create')
            ->setDescription('Create a console super admin')
            ->addArgument('username', Argument::REQUIRED)
            ->addArgument('password', Argument::REQUIRED);
    }

    protected function execute(Input $input, Output $output): int
    {
        $repo = app(AdminRepositoryInterface::class);
        $username = (string) $input->getArgument('username');
        if ($repo->findByUsername($username)) {
            $output->writeln('admin already exists');
            return 1;
        }
        $repo->create([
            'username' => $username,
            'password_hash' => (new PasswordHasher())->hash((string) $input->getArgument('password')),
            'status' => 1,
            'create_time' => date('Y-m-d H:i:s'),
        ]);
        $output->writeln('admin created: ' . $username);
        return 0;
    }
}
