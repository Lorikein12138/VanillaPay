<?php
namespace app\common\service;

use app\common\repository\SettingsRepositoryInterface;
use PHPMailer\PHPMailer\PHPMailer;

final class SmtpMailer implements MailerInterface
{
    public function __construct(private SettingsRepositoryInterface $settings)
    {
    }

    public function send(string $toEmail, string $toName, string $subject, string $html, string $text): void
    {
        $host = trim((string) $this->settings->get('smtp_host', ''));
        $fromEmail = trim((string) $this->settings->get('smtp_from_email', ''));
        if ($host === '' || $fromEmail === '') {
            throw new \RuntimeException('SMTP 参数未配置');
        }

        $mail = new PHPMailer(true);
        $mail->CharSet = 'UTF-8';
        $mail->isSMTP();
        $mail->Host = $host;
        $mail->Port = (int) ($this->settings->get('smtp_port', '587') ?: 587);

        $username = trim((string) $this->settings->get('smtp_username', ''));
        if ($username !== '') {
            $mail->SMTPAuth = true;
            $mail->Username = $username;
            $mail->Password = (string) $this->settings->get('smtp_password', '');
        }

        $secure = (string) $this->settings->get('smtp_secure', 'tls');
        if ($secure === 'ssl') {
            $mail->SMTPSecure = PHPMailer::ENCRYPTION_SMTPS;
        } elseif ($secure === 'tls') {
            $mail->SMTPSecure = PHPMailer::ENCRYPTION_STARTTLS;
        }

        $mail->setFrom($fromEmail, (string) ($this->settings->get('smtp_from_name', 'VanillaPay') ?: 'VanillaPay'));
        $mail->addAddress($toEmail, $toName);
        $mail->Subject = $subject;
        $mail->isHTML(true);
        $mail->Body = $html;
        $mail->AltBody = $text;
        $mail->send();
    }
}
