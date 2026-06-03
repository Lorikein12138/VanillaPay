<?php
use app\common\service\FloatAmountAllocator;
use PHPUnit\Framework\TestCase;

final class FloatAmountAllocatorTest extends TestCase
{
    private FloatAmountAllocator $allocator;

    protected function setUp(): void
    {
        $this->allocator = new FloatAmountAllocator();
    }

    public function test_up_mode(): void
    {
        $this->assertSame([1000, 1001, 1002, 1003], $this->allocator->candidates(1000, 'up', 1, 3));
    }

    public function test_both_interleaves(): void
    {
        $this->assertSame([1000, 1001, 999, 1002, 998], $this->allocator->candidates(1000, 'both', 1, 2));
    }

    public function test_down_never_below_one_cent(): void
    {
        $this->assertSame([2, 1], $this->allocator->candidates(2, 'down', 1, 5));
    }
}
