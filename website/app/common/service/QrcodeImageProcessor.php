<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use GdImage;

final class QrcodeImageProcessor
{
    public function process(string $path, string $mime): void
    {
        if (!extension_loaded('gd')) {
            throw new ValidationException('服务器未安装 GD 扩展，无法处理二维码图片');
        }

        $image = $this->load($path, $mime);
        $bounds = $this->findContentBounds($image);
        if ($bounds === null) {
            throw new ValidationException('未识别到有效二维码内容');
        }

        [$left, $top, $right, $bottom] = $bounds;
        $contentWidth = $right - $left + 1;
        $contentHeight = $bottom - $top + 1;
        $side = max($contentWidth, $contentHeight);
        $quietZone = max(8, (int) round($side * 0.08));
        $outputSide = $side + $quietZone * 2;

        $output = imagecreatetruecolor($outputSide, $outputSide);
        $white = imagecolorallocate($output, 255, 255, 255);
        imagefill($output, 0, 0, $white);

        $sourceX = max(0, $left - (int) floor(($side - $contentWidth) / 2));
        $sourceY = max(0, $top - (int) floor(($side - $contentHeight) / 2));
        imagecopy($output, $image, $quietZone, $quietZone, $sourceX, $sourceY, min($side, imagesx($image) - $sourceX), min($side, imagesy($image) - $sourceY));

        $this->clearCenterAvatar($output);
        $this->save($output, $path, $mime);
    }

    private function load(string $path, string $mime): GdImage
    {
        $image = match ($mime) {
            'image/png' => imagecreatefrompng($path),
            'image/jpeg' => imagecreatefromjpeg($path),
            'image/webp' => function_exists('imagecreatefromwebp') ? imagecreatefromwebp($path) : false,
            'image/gif' => imagecreatefromgif($path),
            default => false,
        };

        if (!$image instanceof GdImage) {
            throw new ValidationException('二维码图片读取失败');
        }

        imagepalettetotruecolor($image);

        return $image;
    }

    private function save(GdImage $image, string $path, string $mime): void
    {
        $saved = match ($mime) {
            'image/png' => imagepng($image, $path, 9),
            'image/jpeg' => imagejpeg($image, $path, 92),
            'image/webp' => function_exists('imagewebp') ? imagewebp($image, $path, 92) : false,
            'image/gif' => imagegif($image, $path),
            default => false,
        };

        if (!$saved) {
            throw new ValidationException('二维码图片保存失败');
        }
    }

    private function findContentBounds(GdImage $image): ?array
    {
        $width = imagesx($image);
        $height = imagesy($image);
        $left = $width;
        $top = $height;
        $right = -1;
        $bottom = -1;

        for ($y = 0; $y < $height; $y++) {
            for ($x = 0; $x < $width; $x++) {
                $rgb = imagecolorsforindex($image, imagecolorat($image, $x, $y));
                if ($rgb['red'] < 245 || $rgb['green'] < 245 || $rgb['blue'] < 245) {
                    $left = min($left, $x);
                    $top = min($top, $y);
                    $right = max($right, $x);
                    $bottom = max($bottom, $y);
                }
            }
        }

        return $right >= $left && $bottom >= $top ? [$left, $top, $right, $bottom] : null;
    }

    private function clearCenterAvatar(GdImage $image): void
    {
        $side = min(imagesx($image), imagesy($image));
        $clearSide = max(10, (int) round($side * 0.16));
        $start = (int) floor(($side - $clearSide) / 2);
        $end = $start + $clearSide;
        $white = imagecolorallocate($image, 255, 255, 255);
        imagefilledrectangle($image, $start, $start, $end, $end, $white);
    }
}
