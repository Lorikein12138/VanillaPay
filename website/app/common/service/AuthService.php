<?php
namespace app\common\service;

use app\common\exception\AuthException;
use app\common\repository\UserRepositoryInterface;

class AuthService
{
    public function __construct(
        private UserRepositoryInterface $users,
        private PasswordHasher $hasher,
        private int $maxFails = 5,
        private int $lockSeconds = 900,
    ) {
    }

    public function login(string $username, string $password, string $ip): array
    {
        $user = $this->users->findByUsername($username);
        if (!$user) {
            throw new AuthException('用户名或密码错误');
        }
        if ((int) ($user['status'] ?? 1) === 2) {
            throw new AuthException('账户已被封禁');
        }
        if (!empty($user['locked_until']) && strtotime($user['locked_until']) > time()) {
            throw new AuthException('账户已锁定，请稍后再试');
        }
        if (!$this->hasher->verify($password, $user['password_hash'])) {
            $fails = (int) ($user['login_fail_count'] ?? 0) + 1;
            $patch = ['login_fail_count' => $fails];
            if ($fails >= $this->maxFails) {
                $patch['locked_until'] = date('Y-m-d H:i:s', time() + $this->lockSeconds);
            }
            $this->users->update((int) $user['id'], $patch);
            throw new AuthException('用户名或密码错误');
        }

        $this->users->update((int) $user['id'], [
            'login_fail_count' => 0,
            'locked_until' => null,
            'last_login_at' => date('Y-m-d H:i:s'),
            'last_login_ip' => $ip,
        ]);

        return $this->users->findById((int) $user['id']);
    }
}
