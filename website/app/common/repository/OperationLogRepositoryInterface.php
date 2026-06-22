<?php
namespace app\common\repository;

interface OperationLogRepositoryInterface
{
    public function record(array $data): void;
}
