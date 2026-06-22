<?php

use app\common\service\QrcodeImageProcessor;
use chillerlan\QRCode\Output\QRGdImagePNG;
use chillerlan\QRCode\QRCode;
use chillerlan\QRCode\QROptions;
use PHPUnit\Framework\TestCase;
use Zxing\QrReader;

final class QrcodeImageProcessorTest extends TestCase
{
    public function testProcessorRegeneratesCleanQrcodeFromWechatRecommendationPoster(): void
    {
        if (!extension_loaded('gd')) {
            $this->markTestSkipped('GD extension is required for QR image processing');
        }

        $dir = sys_get_temp_dir() . '/vp_qr_' . bin2hex(random_bytes(4));
        mkdir($dir);
        $sourceQr = $dir . '/source.png';
        $path = $dir . '/wechat-poster.png';
        $content = 'wxp://f2f0probe/static-test';

        (new QRCode(new QROptions([
            'outputInterface' => QRGdImagePNG::class,
            'outputBase64' => false,
            'eccLevel' => 'H',
            'scale' => 6,
            'quietzoneSize' => 4,
            'imageTransparent' => false,
        ])))->render($content, $sourceQr);
        $this->assertSame($content, (new QrReader($sourceQr, QrReader::SOURCE_TYPE_FILE, false))->text());

        $qr = imagecreatefrompng($sourceQr);
        $this->assertNotFalse($qr);
        $avatarColor = imagecolorallocate($qr, 90, 150, 190);
        $avatarSide = max(12, (int) round(imagesx($qr) * 0.14));
        $avatarStart = (int) floor((imagesx($qr) - $avatarSide) / 2);
        imagefilledrectangle($qr, $avatarStart, $avatarStart, $avatarStart + $avatarSide, $avatarStart + $avatarSide, $avatarColor);

        $image = imagecreatetruecolor(360, 300);
        $white = imagecolorallocate($image, 255, 255, 255);
        $green = imagecolorallocate($image, 0, 190, 96);
        $gray = imagecolorallocate($image, 100, 100, 100);
        imagefill($image, 0, 0, $white);
        imagefilledrectangle($image, 120, 18, 280, 188, $green);
        imagefilledrectangle($image, 150, 58, 250, 158, $white);
        imagecopyresampled($image, $qr, 152, 62, 0, 0, 96, 96, imagesx($qr), imagesy($qr));
        imagestring($image, 4, 144, 214, 'WeChat Pay', $gray);
        imagepng($image, $path);

        $decoded = (new QrcodeImageProcessor())->process($path, 'image/png');
        $this->assertSame($content, $decoded);

        $processed = imagecreatefrompng($path);
        $this->assertNotFalse($processed);
        $this->assertSame(imagesx($processed), imagesy($processed));
        $this->assertGreaterThanOrEqual(220, imagesx($processed));

        $regenerated = (new QrReader($path, QrReader::SOURCE_TYPE_FILE, false))->text();
        $this->assertSame($content, $regenerated);

        $corner = imagecolorsforindex($processed, imagecolorat($processed, 4, 4));
        $this->assertGreaterThan(245, $corner['red']);
        $this->assertGreaterThan(245, $corner['green']);
        $this->assertGreaterThan(245, $corner['blue']);

        unlink($sourceQr);
        unlink($path);
        rmdir($dir);
    }
}
