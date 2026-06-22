<?php
namespace tests\Support;

use app\common\support\Clock;

final class FixedClock implements Clock
{
    public function __construct(private int $ts)
    {
    }

    public function setTs(int $ts): void
    {
        $this->ts = $ts;
    }

    public function now(): string
    {
        return date('Y-m-d H:i:s', $this->ts);
    }

    public function timestamp(): int
    {
        return $this->ts;
    }
}
