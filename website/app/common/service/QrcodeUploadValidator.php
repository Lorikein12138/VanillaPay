<?php
namespace app\common\service;

use app\common\exception\ValidationException;

final class QrcodeUploadValidator
{
    private const ALLOWED = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'];
    private const MAX_BYTES = 5242880;
    private const MAX_SIDE_PIXELS = 4096;
    private const MAX_TOTAL_PIXELS = 12000000;

    /**
     * @param array{0:int,1:int}|false|null $size
     */
    public function validate(string $mime, int $bytes, array|false|null $size = null): void
    {
        if (!in_array($mime, self::ALLOWED, true)) {
            throw new ValidationException('仅支持 PNG/JPEG/WebP/GIF 图片');
        }
        if ($bytes < 1 || $bytes > self::MAX_BYTES) {
            throw new ValidationException('图片大小须在 1B 到 5MB 之间');
        }
        if ($size === false) {
            throw new ValidationException('二维码图片读取失败');
        }
        if (is_array($size)) {
            $width = (int) ($size[0] ?? 0);
            $height = (int) ($size[1] ?? 0);
            if ($width < 1 || $height < 1) {
                throw new ValidationException('二维码图片读取失败');
            }
            if ($width > self::MAX_SIDE_PIXELS || $height > self::MAX_SIDE_PIXELS || $width * $height > self::MAX_TOTAL_PIXELS) {
                throw new ValidationException('图片尺寸过大，请上传 4096px 以内的收款码图片');
            }
        }
    }

    public function validateChannel(string $channel): void
    {
        if (!in_array($channel, ['wxpay', 'alipay'], true)) {
            throw new ValidationException('支付渠道不支持');
        }
    }
}
