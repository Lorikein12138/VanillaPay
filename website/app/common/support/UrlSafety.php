<?php
namespace app\common\support;

final class UrlSafety
{
    public static function assertPublicHttpUrl(string $url, string $message = 'URL 不合法'): void
    {
        $parts = parse_url($url);
        $scheme = strtolower((string) ($parts['scheme'] ?? ''));
        $host = strtolower(trim((string) ($parts['host'] ?? ''), '[]'));
        if (!$parts || !in_array($scheme, ['http', 'https'], true) || $host === '') {
            throw new \InvalidArgumentException($message);
        }
        if (self::isBlockedHost($host)) {
            throw new \InvalidArgumentException('URL 不允许使用内网地址');
        }
    }

    public static function assertPublicHttpTarget(string $url, string $message = 'URL 不合法'): void
    {
        self::assertPublicHttpUrl($url, $message);
        $host = strtolower(trim((string) (parse_url($url, PHP_URL_HOST) ?? ''), '[]'));
        $ips = self::resolveHost($host);
        if ($ips === []) {
            throw new \InvalidArgumentException('URL 无法解析');
        }
        foreach ($ips as $ip) {
            if (self::isBlockedIp($ip)) {
                throw new \InvalidArgumentException('URL 解析到内网地址');
            }
        }
    }

    private static function isBlockedHost(string $host): bool
    {
        if ($host === 'localhost' || str_ends_with($host, '.localhost') || str_ends_with($host, '.local')) {
            return true;
        }
        return filter_var($host, FILTER_VALIDATE_IP) && self::isBlockedIp($host);
    }

    private static function isBlockedIp(string $ip): bool
    {
        return filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE) === false;
    }

    /**
     * @return list<string>
     */
    private static function resolveHost(string $host): array
    {
        if (filter_var($host, FILTER_VALIDATE_IP)) {
            return [$host];
        }

        $records = @dns_get_record($host, DNS_A + DNS_AAAA);
        if (!$records) {
            return [];
        }

        $ips = [];
        foreach ($records as $record) {
            foreach (['ip', 'ipv6'] as $key) {
                if (!empty($record[$key]) && filter_var($record[$key], FILTER_VALIDATE_IP)) {
                    $ips[] = $record[$key];
                }
            }
        }

        return array_values(array_unique($ips));
    }
}
