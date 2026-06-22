<?php
use app\common\support\Money;
use PHPUnit\Framework\TestCase;

final class MoneyTest extends TestCase
{
    public function test_to_cents(): void
    {
        $this->assertSame(1000, Money::toCents('10.00'));
        $this->assertSame(999, Money::toCents('9.99'));
        $this->assertSame(10, Money::toCents(0.1));
    }

    public function test_from_cents(): void
    {
        $this->assertSame('10.03', Money::fromCents(1003));
        $this->assertSame('0.01', Money::fromCents(1));
    }
}
