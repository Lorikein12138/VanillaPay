<?php
namespace app\common\service;

final class DeviceSigner
{
    public function sign(array $params, string $key): string
    {
        unset($params['sign']);
        $params = array_filter($params, fn ($value): bool => $value !== '' && $value !== null);
        ksort($params);

        $pairs = [];
        foreach ($params as $name => $value) {
            $pairs[] = $name . '=' . $value;
        }

        return hash_hmac('sha256', implode('&', $pairs), $key);
    }

    public function verify(array $params, string $key): bool
    {
        $sign = (string) ($params['sign'] ?? '');
        return $sign !== '' && hash_equals($this->sign($params, $key), $sign);
    }

    public function timestampValid(int $timestamp, int $now, int $windowSeconds = 300): bool
    {
        return abs($now - $timestamp) <= $windowSeconds;
    }
}
