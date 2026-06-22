<?php

use PHPUnit\Framework\TestCase;

final class EmailVerificationFeatureTest extends TestCase
{
    public function testAuthControllerUsesEmailCodesForRegisterAndPasswordReset(): void
    {
        $controller = file_get_contents(dirname(__DIR__, 2) . '/app/index/controller/Auth.php') ?: '';
        $route = file_get_contents(dirname(__DIR__, 2) . '/route/index.php') ?: '';

        $this->assertStringContainsString("Route::post('register/code'", $route);
        $this->assertStringContainsString('sendRegisterCode', $controller);
        $this->assertStringContainsString('EmailVerificationService', $controller);
        $this->assertStringContainsString('private const CODE_COOLDOWN_SECONDS = 60', $controller);
        $this->assertStringContainsString('ensureEmailCodeCooldown', $controller);
        $this->assertStringContainsString('markEmailCodeSent', $controller);
        $this->assertStringContainsString('emailCodeCooldownRemaining', $controller);
        $this->assertStringContainsString("email_code_sent_at_register", $controller);
        $this->assertStringContainsString("email_code_sent_at_reset", $controller);
        $this->assertStringContainsString("Session::get('register_email_verification'", $controller);
        $this->assertStringContainsString("Session::get('reset_email_verification'", $controller);
        $this->assertStringContainsString('email_code', $controller);
        $this->assertStringContainsString("redirect('/forgot?email='", $controller);
        $this->assertStringNotContainsString("View::fetch('auth/reset'", $controller);
        $this->assertStringNotContainsString('trace(\'reset link:', $controller);
        $this->assertStringNotContainsString('PasswordResetService', $controller);
        $this->assertStringNotContainsString('/reset?token=', $controller);
    }

    public function testMailServicesAndTemplateAreStyledHtmlEmail(): void
    {
        $mailer = file_get_contents(dirname(__DIR__, 2) . '/app/common/service/SmtpMailer.php') ?: '';
        $verification = file_get_contents(dirname(__DIR__, 2) . '/app/common/service/EmailVerificationService.php') ?: '';
        $template = file_get_contents(dirname(__DIR__, 2) . '/app/common/service/EmailTemplateRenderer.php') ?: '';
        $provider = file_get_contents(dirname(__DIR__, 2) . '/app/provider.php') ?: '';

        foreach ([
            'smtp_host',
            'smtp_port',
            'smtp_secure',
            'smtp_username',
            'smtp_password',
            'smtp_from_email',
            'smtp_from_name',
        ] as $key) {
            $this->assertStringContainsString($key, $mailer);
        }

        $this->assertStringContainsString('PHPMailer\\PHPMailer\\PHPMailer', $mailer);
        $this->assertStringContainsString('isHTML(true)', $mailer);
        $this->assertStringContainsString('EmailTemplateRenderer::verificationCode', $verification);
        $this->assertStringContainsString('background:#f3f7f6', $template);
        $this->assertStringContainsString('border-radius:18px', $template);
        $this->assertStringContainsString('background:#ecfdf5', $template);
        $this->assertStringContainsString('color:#0f172a', $template);
        $this->assertStringContainsString('VanillaPay', $template);
        $this->assertStringContainsString('验证码', $template);
        $this->assertStringNotContainsString('linear-gradient(135deg,#0f172a,#0f766e)', $template);
        $this->assertStringNotContainsString('color:#ffffff;">{$safeCode}', $template);
        $this->assertStringContainsString('MailerInterface::class', $provider);
        $this->assertStringContainsString('SmtpMailer::class', $provider);
    }
}
