<?php
namespace app\common\support;

final class Money
{
    public static function toCents(string|float|int $amount): int
    {
        return (int) round(((float) $amount) * 100);
    }

    public static function fromCents(int $cents): string
    {
        return number_format($cents / 100, 2, '.', '');
    }
}
