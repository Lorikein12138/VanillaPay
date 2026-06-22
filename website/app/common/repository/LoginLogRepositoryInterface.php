<?php
namespace app\common\repository;

interface LoginLogRepositoryInterface
{
    public function record(array $data): void;
}
