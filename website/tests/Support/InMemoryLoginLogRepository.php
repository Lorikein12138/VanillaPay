<?php
namespace tests\Support;

use app\common\repository\LoginLogRepositoryInterface;

final class InMemoryLoginLogRepository implements LoginLogRepositoryInterface
{
    public array $rows = [];

    public function record(array $data): void
    {
        $this->rows[] = $data;
    }
}
