<?php
namespace app\common\support;

final class SystemClock implements Clock
{
    public function now(): string
    {
        return date('Y-m-d H:i:s');
    }

    public function timestamp(): int
    {
        return time();
    }
}
