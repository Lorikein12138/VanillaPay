<?php
namespace app\common\service;

class PasswordResetService
{
    public function __construct(private string $secret, private int $ttl = 3600)
    {
    }

    public function issue(int $userId): string
    {
        $exp = time() + $this->ttl;
        $payload = $userId . '.' . $exp;
        $sig = hash_hmac('sha256', $payload, $this->secret);
        return rtrim(strtr(base64_encode($payload . '.' . $sig), '+/', '-_'), '=');
    }

    public function verify(string $token): ?int
    {
        $raw = base64_decode(strtr($token, '-_', '+/'), true);
        if ($raw === false) {
            return null;
        }
        $parts = explode('.', $raw);
        if (count($parts) !== 3) {
            return null;
        }
        [$userId, $exp, $sig] = $parts;
        $expected = hash_hmac('sha256', $userId . '.' . $exp, $this->secret);
        if (!hash_equals($expected, $sig)) {
            return null;
        }
        if ((int) $exp < time()) {
            return null;
        }
        return (int) $userId;
    }
}
