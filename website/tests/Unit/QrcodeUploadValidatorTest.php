<?php

use app\common\exception\ValidationException;
use app\common\service\QrcodeUploadValidator;
use PHPUnit\Framework\TestCase;

final class QrcodeUploadValidatorTest extends TestCase
{
    public function test_rejects_untrusted_mime_type(): void
    {
        $this->expectException(ValidationException::class);
        $this->expectExceptionMessage('仅支持 PNG/JPEG/WebP/GIF 图片');

        (new QrcodeUploadValidator())->validate('text/plain', 100, [100, 100]);
    }

    public function test_rejects_large_pixel_dimensions_before_image_processing(): void
    {
        $this->expectException(ValidationException::class);
        $this->expectExceptionMessage('图片尺寸过大');

        (new QrcodeUploadValidator())->validate('image/png', 100, [5000, 5000]);
    }
}
