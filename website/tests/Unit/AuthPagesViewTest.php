<?php

use PHPUnit\Framework\TestCase;

final class AuthPagesViewTest extends TestCase
{
    public function testLoginPageUsesModernTailwindAuthLayout(): void
    {
        $template = $this->template('login');

        $this->assertStringContainsString('/static/brand/VanillaClub.png', $template);
        $this->assertStringContainsString('lg:grid-cols-[minmax(0,0.95fr)_minmax(420px,0.62fr)]', $template);
        $this->assertStringContainsString('bg-gradient-to-br', $template);
        $this->assertStringContainsString('backdrop-blur-xl', $template);
        $this->assertStringContainsString('shadow-2xl', $template);
        $this->assertStringContainsString('商户身份验证', $template);
        $this->assertStringContainsString('收款工作台', $template);

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
        $this->assertStringContainsString('lg:grid-cols-[minmax(0,0.95fr)_minmax(440px,0.62fr)]', $template);
        $this->assertStringContainsString('bg-gradient-to-br', $template);
        $this->assertStringContainsString('backdrop-blur-xl', $template);
        $this->assertStringContainsString('shadow-2xl', $template);
        $this->assertStringContainsString('创建商户账户', $template);
        $this->assertStringContainsString('收款工作台', $template);

        $this->assertStringContainsString('method="post" action="/register"', $template);
        $this->assertStringContainsString('name="_csrf"', $template);
        $this->assertStringContainsString('name="username"', $template);
        $this->assertStringContainsString('name="email"', $template);
        $this->assertStringContainsString('name="password"', $template);
        $this->assertStringContainsString('href="/login"', $template);

        $this->assertNoRuntimeDetails($template);
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
