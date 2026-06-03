<?php
namespace app\common\service;

use app\common\exception\ValidationException;

final class QrcodeUploadValidator
{
    private const ALLOWED = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'];
    private const MAX_BYTES = 5242880;

    public function validate(string $mime, int $bytes): void
    {
        if (!in_array($mime, self::ALLOWED, true)) {
            throw new ValidationException('仅支持 PNG/JPEG/WebP/GIF 图片');
        }
        if ($bytes < 1 || $bytes > self::MAX_BYTES) {
            throw new ValidationException('图片大小须在 1B 到 5MB 之间');
        }
    }

    public function validateChannel(string $channel): void
    {
        if (!in_array($channel, ['wxpay', 'alipay'], true)) {
            throw new ValidationException('支付渠道不支持');
        }
    }
}
