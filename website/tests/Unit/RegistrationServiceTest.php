<?php
use PHPUnit\Framework\TestCase;
use app\common\service\{RegistrationService, PasswordHasher, CredentialGenerator};
use app\common\exception\ValidationException;
use tests\Support\InMemoryUserRepository;

final class RegistrationServiceTest extends TestCase
{
    private function svc(InMemoryUserRepository $repo): RegistrationService
    {
        return new RegistrationService($repo, new PasswordHasher(), new CredentialGenerator());
    }

    public function test_registers_user_with_pid_apikey_and_hashed_password(): void
    {
        $repo = new InMemoryUserRepository();
        $id = $this->svc($repo)->register('alice', 'alice@example.com', 'S3cret!pw');
        $row = $repo->findById($id);
        $this->assertSame('alice', $row['username']);
        $this->assertNotSame('S3cret!pw', $row['password_hash']);
        $this->assertMatchesRegularExpression('/^\d{6,12}$/', $row['pid']);
        $this->assertMatchesRegularExpression('/^[a-f0-9]{32}$/', $row['api_key']);
        $this->assertSame(1, $row['status']);
    }

    public function test_rejects_duplicate_username(): void
    {
        $repo = new InMemoryUserRepository();
        $this->svc($repo)->register('alice', 'a@example.com', 'S3cret!pw');
        $this->expectException(ValidationException::class);
        $this->svc($repo)->register('alice', 'b@example.com', 'S3cret!pw');
    }

    public function test_rejects_invalid_email(): void
    {
        $this->expectException(ValidationException::class);
        $this->svc(new InMemoryUserRepository())->register('bob', 'not-an-email', 'S3cret!pw');
    }

    public function test_rejects_short_password(): void
    {
        $this->expectException(ValidationException::class);
        $this->svc(new InMemoryUserRepository())->register('bob', 'b@example.com', '123');
    }
}
