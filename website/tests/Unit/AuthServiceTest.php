<?php
use PHPUnit\Framework\TestCase;
use app\common\service\{AuthService, PasswordHasher};
use app\common\exception\AuthException;
use tests\Support\InMemoryUserRepository;

final class AuthServiceTest extends TestCase
{
    private InMemoryUserRepository $repo;
    private AuthService $auth;

    protected function setUp(): void
    {
        $this->repo = new InMemoryUserRepository();
        $hasher = new PasswordHasher();
        $this->repo->create([
            'username' => 'alice',
            'email' => 'a@e.com',
            'password_hash' => $hasher->hash('GoodPass1'),
            'status' => 1,
            'login_fail_count' => 0,
            'locked_until' => null,
        ]);
        $this->auth = new AuthService($this->repo, $hasher, maxFails: 5, lockSeconds: 900);
    }

    public function test_login_success_returns_user_and_resets_fail_count(): void
    {
        $user = $this->auth->login('alice', 'GoodPass1', '1.2.3.4');
        $this->assertSame('alice', $user['username']);
        $this->assertSame(0, $this->repo->findById(1)['login_fail_count']);
    }

    public function test_wrong_password_increments_fail_count(): void
    {
        try {
            $this->auth->login('alice', 'bad', '1.2.3.4');
        } catch (AuthException) {
        }
        $this->assertSame(1, $this->repo->findById(1)['login_fail_count']);
    }

    public function test_locks_after_max_fails(): void
    {
        for ($i = 0; $i < 5; $i++) {
            try {
                $this->auth->login('alice', 'bad', '1.2.3.4');
            } catch (AuthException) {
            }
        }
        $this->expectException(AuthException::class);
        $this->expectExceptionMessage('锁定');
        $this->auth->login('alice', 'GoodPass1', '1.2.3.4');
    }

    public function test_banned_user_cannot_login(): void
    {
        $this->repo->update(1, ['status' => 2]);
        $this->expectException(AuthException::class);
        $this->auth->login('alice', 'GoodPass1', '1.2.3.4');
    }
}
