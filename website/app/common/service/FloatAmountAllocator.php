<?php
namespace app\common\service;

final class FloatAmountAllocator
{
    /**
     * @return int[]
     */
    public function candidates(int $baseCents, string $mode, int $stepCents, int $maxCents): array
    {
        $mode = in_array($mode, ['up', 'down', 'both'], true) ? $mode : 'up';
        $stepCents = max(1, $stepCents);
        $maxCents = min(max(0, $maxCents), 100);
        $steps = intdiv($maxCents, $stepCents);

        $list = [$baseCents];
        for ($i = 1; $i <= $steps; $i++) {
            $delta = $i * $stepCents;
            if ($mode === 'up' || $mode === 'both') {
                $list[] = $baseCents + $delta;
            }
            if ($mode === 'down' || $mode === 'both') {
                $down = $baseCents - $delta;
                if ($down >= 1) {
                    $list[] = $down;
                }
            }
        }

        $seen = [];
        $out = [];
        foreach ($list as $cents) {
            if ($cents >= 1 && !isset($seen[$cents])) {
                $seen[$cents] = true;
                $out[] = $cents;
            }
        }
        return $out;
    }
}
