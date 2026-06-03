<?php

use app\common\service\QrcodeImageProcessor;
use PHPUnit\Framework\TestCase;

final class QrcodeImageProcessorTest extends TestCase
{
    public function testProcessorCropsOuterBorderAndClearsCenterAvatar(): void
    {
        if (!extension_loaded('gd')) {
            $this->markTestSkipped('GD extension is required for QR image processing');
        }

        $dir = sys_get_temp_dir() . '/vp_qr_' . bin2hex(random_bytes(4));
        mkdir($dir);
        $path = $dir . '/qr.png';

        $image = imagecreatetruecolor(120, 120);
        $white = imagecolorallocate($image, 255, 255, 255);
        $black = imagecolorallocate($image, 0, 0, 0);
        $red = imagecolorallocate($image, 220, 40, 40);
        imagefill($image, 0, 0, $white);
        imagefilledrectangle($image, 30, 30, 89, 89, $black);
        imagefilledrectangle($image, 51, 51, 68, 68, $red);
        imagepng($image, $path);

        (new QrcodeImageProcessor())->process($path, 'image/png');

        $processed = imagecreatefrompng($path);
        $this->assertNotFalse($processed);
        $this->assertLessThan(120, imagesx($processed));
        $this->assertSame(imagesx($processed), imagesy($processed));

        $center = imagecolorat($processed, intdiv(imagesx($processed), 2), intdiv(imagesy($processed), 2));
        $rgb = imagecolorsforindex($processed, $center);
        $this->assertGreaterThan(245, $rgb['red']);
        $this->assertGreaterThan(245, $rgb['green']);
        $this->assertGreaterThan(245, $rgb['blue']);

        unlink($path);
        rmdir($dir);
    }
}
