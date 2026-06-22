<?php
namespace app\common\service;

interface MailerInterface
{
    public function send(string $toEmail, string $toName, string $subject, string $html, string $text): void;
}
