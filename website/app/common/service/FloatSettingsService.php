<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use app\common\support\Money;

final class FloatSettingsService
{
    public function validate(string $mode, string $step, string $max, int $timeout): array
    {
        if (!in_array($mode, ['up', 'down', 'both'], true)) {
            throw new ValidationException('浮动方向不合法');
        }
        $stepCents = Money::toCents($step);
        $maxCents = Money::toCents($max);
        if ($stepCents < 1) {
            throw new ValidationException('浮动步长必须大于 0');
        }
        if ($maxCents < 1 || $maxCents > 100) {
            throw new ValidationException('浮动上限须在 0.01 ~ 1.00 元之间');
        }
        if ($stepCents > $maxCents) {
            throw new ValidationException('浮动步长不能大于浮动上限');
        }
        if ($timeout < 60 || $timeout > 1800) {
            throw new ValidationException('订单有效期须在 60 ~ 1800 秒之间');
        }

        return [
            'float_mode' => $mode,
            'float_step' => Money::fromCents($stepCents),
            'float_max' => Money::fromCents($maxCents),
            'order_timeout' => $timeout,
        ];
    }
}
