<?php

use PHPUnit\Framework\TestCase;

final class AuthPagesViewTest extends TestCase
{
    public function testLoginPageUsesModernTailwindAuthLayout(): void
    {
        $template = $this->template('login');

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $template);
        $this->assertStringContainsString('max-w-md', $template);
        $this->assertStringContainsString('bg-gradient-to-br', $template);
        $this->assertStringContainsString('backdrop-blur-xl', $template);
        $this->assertStringContainsString('shadow-2xl', $template);
        $this->assertStringContainsString('登录商户', $template);
        $this->assertStringContainsString('创建商户', $template);
        $this->assertStringNotContainsString('<aside', $template);
        $this->assertStringNotContainsString('商户身份验证', $template);
        $this->assertStringNotContainsString('登录收款工作台', $template);
        $this->assertStringNotContainsString('使用商户账号登录，继续处理订单、设备和回调。', $template);
        $this->assertStringNotContainsString('创建商户账户', $template);

        $this->assertStringContainsString('method="post" action="/login"', $template);
        $this->assertStringContainsString('name="_csrf"', $template);
        $this->assertStringContainsString('name="username"', $template);
        $this->assertStringContainsString('name="password"', $template);
        $this->assertStringContainsString('href="/register"', $template);
        $this->assertStringContainsString('href="/forgot"', $template);

        $this->assertNoRuntimeDetails($template);
    }

    public function testRegisterPageUsesModernTailwindAuthLayout(): void
    {
        $template = $this->template('register');

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $template);
        $this->assertStringContainsString('max-w-md', $template);
        $this->assertStringContainsString('bg-gradient-to-br', $template);
        $this->assertStringContainsString('backdrop-blur-xl', $template);
        $this->assertStringContainsString('shadow-2xl', $template);
        $this->assertStringContainsString('创建商户', $template);
        $this->assertStringContainsString('>登录</a>', $template);
        $this->assertStringNotContainsString('<aside', $template);
        $this->assertStringNotContainsString('收款工作台', $template);
        $this->assertStringNotContainsString('创建商户账户', $template);
        $this->assertStringNotContainsString('填写基础账号信息，注册后进入商户中心继续配置。', $template);
        $this->assertStringNotContainsString('返回首页', $template);
        $this->assertStringNotContainsString('已有账号，登录', $template);
        $this->assertStringNotContainsString('text-teal-700 hover:text-teal-900">登录</a>', $template);

        $this->assertStringContainsString('method="post" action="/register"', $template);
        $this->assertStringContainsString('name="_csrf"', $template);
        $this->assertStringContainsString('name="username"', $template);
        $this->assertStringContainsString('name="email"', $template);
        $this->assertStringContainsString('name="email_code"', $template);
        $this->assertStringContainsString('formaction="/register/code"', $template);
        $this->assertStringContainsString('发送验证码', $template);
        $this->assertStringContainsString('name="password"', $template);
        $this->assertStringContainsString('href="/login"', $template);

        $this->assertNoRuntimeDetails($template);
    }

    public function testForgotPageCombinesCodeSendingAndPasswordReset(): void
    {
        $forgot = $this->template('forgot');

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $forgot);
        $this->assertStringContainsString('bg-gradient-to-br', $forgot);
        $this->assertStringContainsString('backdrop-blur-xl', $forgot);
        $this->assertStringContainsString('shadow-2xl', $forgot);

        $this->assertStringContainsString('发送验证码', $forgot);
        $this->assertStringContainsString('重置密码', $forgot);
        $this->assertStringContainsString('method="post" action="/reset"', $forgot);
        $this->assertStringContainsString('formaction="/forgot"', $forgot);
        $this->assertStringContainsString('name="email"', $forgot);
        $this->assertStringContainsString('name="email_code"', $forgot);
        $this->assertStringContainsString('name="password"', $forgot);
        $this->assertStringNotContainsString('href="/reset"', $forgot);
        $this->assertStringNotContainsString('name="token"', $forgot);
        $this->assertStringNotContainsString('{$token}', $forgot);
    }

    private function template(string $page): string
    {
        return file_get_contents(dirname(__DIR__, 2) . "/view/index/auth/{$page}.html") ?: '';
    }

    private function assertNoRuntimeDetails(string $template): void
    {
        foreach (['MySQL 8.4', '/www/', 'wwwroot', 'lorikein', '154.89', 'root@'] as $text) {
            $this->assertStringNotContainsString($text, $template);
        }
    }
}
