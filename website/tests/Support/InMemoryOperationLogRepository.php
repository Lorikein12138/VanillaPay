<?php
namespace tests\Support;

use app\common\repository\OperationLogRepositoryInterface;

final class InMemoryOperationLogRepository implements OperationLogRepositoryInterface
{
    public array $rows = [];

    public function record(array $data): void
    {
        $this->rows[] = $data;
    }
}
