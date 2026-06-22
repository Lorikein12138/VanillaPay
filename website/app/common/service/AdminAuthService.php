<?php
namespace app\common\service;

use app\common\exception\AuthException;
use app\common\repository\AdminRepositoryInterface;

final class AdminAuthService
{
    public function __construct(private AdminRepositoryInterface $admins, private PasswordHasher $hasher)
    {
    }

    public function login(string $username, string $password, string $ip): array
    {
        $admin = $this->admins->findByUsername($username);
        if (!$admin || (int) ($admin['status'] ?? 0) !== 1 || !$this->hasher->verify($password, (string) $admin['password_hash'])) {
            throw new AuthException('账号或密码错误');
        }
        $this->admins->update((int) $admin['id'], [
            'last_login_at' => date('Y-m-d H:i:s'),
            'last_login_ip' => $ip,
        ]);
        return $this->admins->findById((int) $admin['id']);
    }
}
