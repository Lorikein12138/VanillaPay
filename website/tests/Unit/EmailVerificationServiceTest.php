<?php

use app\common\service\EmailVerificationService;
use app\common\service\MailerInterface;
use PHPUnit\Framework\TestCase;
use tests\Support\FixedClock;

final class EmailVerificationServiceTest extends TestCase
{
    public function testCodeCanBeSentAndVerifiedBeforeExpiry(): void
    {
        $mailer = new CapturingMailer();
        $clock = new FixedClock(1780812000);
        $service = new EmailVerificationService($mailer, $clock);

        $record = $service->sendCode('注册', 'USER@example.com');
        $code = $mailer->lastCode();

        $this->assertSame('user@example.com', $record['email']);
        $this->assertSame('user@example.com', $mailer->toEmail);
        $this->assertStringContainsString('验证码', $mailer->html);
        $this->assertTrue($service->verify($record, 'user@example.com', $code));
        $this->assertFalse($service->verify($record, 'other@example.com', $code));
        $this->assertFalse($service->verify($record, 'user@example.com', '000000'));

        $clock->setTs(1780812000 + 601);
        $this->assertFalse($service->verify($record, 'user@example.com', $code));
    }
}

final class CapturingMailer implements MailerInterface
{
    public string $toEmail = '';
    public string $html = '';
    public string $text = '';

    public function send(string $toEmail, string $toName, string $subject, string $html, string $text): void
    {
        $this->toEmail = $toEmail;
        $this->html = $html;
        $this->text = $text;
    }

    public function lastCode(): string
    {
        if (preg_match('/\b(\d{6})\b/', $this->text, $matches) !== 1) {
            throw new RuntimeException('Verification code was not captured.');
        }

        return $matches[1];
    }
}
