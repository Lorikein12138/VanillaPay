<?php
namespace app\common\support;

final class SignHelper
{
    public static function epayStyle(array $params, string $key, array $exclude = ['sign', 'sign_type']): string
    {
        foreach ($exclude as $name) {
            unset($params[$name]);
        }

        $params = array_filter($params, fn ($value): bool => $value !== '' && $value !== null);
        ksort($params);

        $pairs = [];
        foreach ($params as $name => $value) {
            $pairs[] = $name . '=' . $value;
        }

        return md5(implode('&', $pairs) . $key);
    }
}
