<?php
namespace app\common\service;

class CredentialGenerator
{
    public function pid(): string
    {
        return (string) random_int(1_000_000_000, 9_999_999_999);
    }

    public function apiKey(): string
    {
        return bin2hex(random_bytes(16));
    }
}
