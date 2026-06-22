<?php
namespace app\common\support;

interface Clock
{
    public function now(): string;
    public function timestamp(): int;
}
