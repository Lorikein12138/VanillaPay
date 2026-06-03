<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use app\common\repository\UserRepositoryInterface;

class RegistrationService
{
    public function __construct(
        private UserRepositoryInterface $users,
        private PasswordHasher $hasher,
        private CredentialGenerator $cred,
    ) {
    }

    public function register(string $username, string $email, string $password): int
    {
        if (!preg_match('/^[A-Za-z0-9_]{3,50}$/', $username)) {
            throw new ValidationException('用户名需为 3-50 位字母数字下划线');
        }
        if (!filter_var($email, FILTER_VALIDATE_EMAIL)) {
            throw new ValidationException('邮箱格式不正确');
        }
        if (strlen($password) < 8) {
            throw new ValidationException('密码至少 8 位');
        }
        if ($this->users->existsUsername($username)) {
            throw new ValidationException('用户名已存在');
        }
        if ($this->users->existsEmail($email)) {
            throw new ValidationException('邮箱已被注册');
        }

        $now = date('Y-m-d H:i:s');
        return $this->users->create([
            'username' => $username,
            'email' => $email,
            'password_hash' => $this->hasher->hash($password),
            'pid' => $this->cred->pid(),
            'api_key' => $this->cred->apiKey(),
            'status' => 1,
            'float_mode' => 'up',
            'float_step' => 0.01,
            'float_max' => 0.10,
            'order_timeout' => 300,
            'create_time' => $now,
            'update_time' => $now,
        ]);
    }
}
