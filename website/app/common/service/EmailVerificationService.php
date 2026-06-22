<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use app\common\support\Clock;

final class EmailVerificationService
{
    private const TTL_SECONDS = 600;

    public function __construct(private MailerInterface $mailer, private Clock $clock)
    {
    }

    public function sendCode(string $scene, string $email): array
    {
        $email = strtolower(trim($email));
        if (!filter_var($email, FILTER_VALIDATE_EMAIL)) {
            throw new ValidationException('邮箱格式不正确');
        }

        $code = (string) random_int(100000, 999999);
        $message = EmailTemplateRenderer::verificationCode($scene, $code, (int) (self::TTL_SECONDS / 60));
        $this->mailer->send($email, '', $message['subject'], $message['html'], $message['text']);

        return [
            'email' => $email,
            'code_hash' => password_hash($code, PASSWORD_DEFAULT),
            'expires_at' => $this->clock->timestamp() + self::TTL_SECONDS,
        ];
    }

    public function verify(?array $record, string $email, string $code): bool
    {
        if (!$record) {
            return false;
        }

        $email = strtolower(trim($email));
        $code = trim($code);
        if ($email === '' || $code === '') {
            return false;
        }

        if (($record['email'] ?? '') !== $email) {
            return false;
        }
        if ((int) ($record['expires_at'] ?? 0) < $this->clock->timestamp()) {
            return false;
        }

        return password_verify($code, (string) ($record['code_hash'] ?? ''));
    }
}
