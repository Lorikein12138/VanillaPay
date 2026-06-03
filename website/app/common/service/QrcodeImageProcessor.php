<?php
namespace app\common\service;

use app\common\exception\ValidationException;
use chillerlan\QRCode\Output\QRGdImagePNG;
use chillerlan\QRCode\QRCode;
use chillerlan\QRCode\QROptions;
use GdImage;
use Zxing\QrReader;

final class QrcodeImageProcessor
{
    public function process(string $path, string $mime): string
    {
        if (!extension_loaded('gd')) {
            throw new ValidationException('服务器未安装 GD 扩展，无法处理二维码图片');
        }

        $content = $this->decode($path, $mime);
        if ($content === '') {
            throw new ValidationException('未识别到有效二维码内容，请上传微信或支付宝收款码原图');
        }

        $this->renderCleanQrcode($content, $path);

        return $content;
    }

    private function decode(string $path, string $mime): string
    {
        $content = $this->decodeFile($path);
        if ($content !== '') {
            return $content;
        }

        $image = $this->load($path, $mime);
        $candidates = $this->candidateBounds($image);
        foreach ($candidates as $bounds) {
            $candidate = $this->cropCandidate($image, $bounds);
            $content = $this->decodeImageVariants($candidate);
            if ($content !== '') {
                return $content;
            }

            $this->clearCenterAvatar($candidate);
            $content = $this->decodeImageVariants($candidate);
            if ($content !== '') {
                return $content;
            }
        }

        return '';
    }

    private function decodeFile(string $path): string
    {
        try {
            $text = (new QrReader($path, QrReader::SOURCE_TYPE_FILE, false))->text();
            return is_string($text) ? trim($text) : '';
        } catch (\Throwable) {
            return '';
        }
    }

    private function decodeImage(GdImage $image): string
    {
        try {
            $text = (new QrReader($image, QrReader::SOURCE_TYPE_RESOURCE, false))->text();
            return is_string($text) ? trim($text) : '';
        } catch (\Throwable) {
            return '';
        }
    }

    private function decodeImageVariants(GdImage $image): string
    {
        foreach ([1, 2, 3, 4] as $scale) {
            $variant = $scale === 1
                ? $image
                : imagescale($image, imagesx($image) * $scale, imagesy($image) * $scale, IMG_NEAREST_NEIGHBOUR);
            if (!$variant instanceof GdImage) {
                continue;
            }
            $content = $this->decodeImage($variant);
            if ($content !== '') {
                return $content;
            }
        }

        return '';
    }

    private function renderCleanQrcode(string $content, string $path): void
    {
        (new QRCode(new QROptions([
            'outputInterface' => QRGdImagePNG::class,
            'outputBase64' => false,
            'eccLevel' => 'H',
            'scale' => 8,
            'quietzoneSize' => 4,
            'imageTransparent' => false,
            'quality' => 90,
        ])))->render($content, $path);
    }

    private function load(string $path, string $mime): GdImage
    {
        $image = match ($mime) {
            'image/png' => imagecreatefrompng($path),
            'image/jpeg' => imagecreatefromjpeg($path),
            'image/webp' => function_exists('imagecreatefromwebp') ? imagecreatefromwebp($path) : false,
            'image/gif' => imagecreatefromgif($path),
            default => imagecreatefromstring((string) file_get_contents($path)),
        };

        if (!$image instanceof GdImage) {
            throw new ValidationException('二维码图片读取失败');
        }

        imagepalettetotruecolor($image);

        return $image;
    }

    /**
     * Finds square-ish high contrast regions. This avoids treating WeChat/Alipay
     * poster backgrounds and labels as QR content.
     *
     * @return array<int,array{0:int,1:int,2:int,3:int}>
     */
    private function candidateBounds(GdImage $image): array
    {
        $width = imagesx($image);
        $height = imagesy($image);
        $visited = array_fill(0, $height, array_fill(0, $width, false));
        $components = [];

        for ($y = 0; $y < $height; $y++) {
            for ($x = 0; $x < $width; $x++) {
                if ($visited[$y][$x] || !$this->isDarkPixel($image, $x, $y)) {
                    $visited[$y][$x] = true;
                    continue;
                }

                $component = $this->floodFill($image, $visited, $x, $y);
            if ($component['pixels'] < 8) {
                continue;
            }

            $components[] = $component;
            }
        }

        $clusters = [];
        foreach ($components as $component) {
            [$left, $top, $right, $bottom] = $component['bounds'];
            $side = max($right - $left + 1, $bottom - $top + 1);
            $padding = max(6, (int) round($side * 0.45));
            $clusters[] = [
                max(0, $left - $padding),
                max(0, $top - $padding),
                min($width - 1, $right + $padding),
                min($height - 1, $bottom + $padding),
            ];
        }

        usort($clusters, fn (array $a, array $b): int => $this->area($b) <=> $this->area($a));

        $merged = [];
        foreach ($clusters as $cluster) {
            $added = false;
            foreach ($merged as $idx => $existing) {
                if ($this->intersects($existing, $cluster)) {
                    $merged[$idx] = [
                        min($existing[0], $cluster[0]),
                        min($existing[1], $cluster[1]),
                        max($existing[2], $cluster[2]),
                        max($existing[3], $cluster[3]),
                    ];
                    $added = true;
                    break;
                }
            }
            if (!$added) {
                $merged[] = $cluster;
            }
        }

        $candidates = [];
        foreach ($merged as $bounds) {
            [$left, $top, $right, $bottom] = $bounds;
            $candidateWidth = $right - $left + 1;
            $candidateHeight = $bottom - $top + 1;
            $ratio = $candidateWidth / max(1, $candidateHeight);
            if ($candidateWidth < 40 || $candidateHeight < 40 || $ratio < 0.55 || $ratio > 1.8) {
                continue;
            }
            $candidates[] = $this->squareBounds($bounds, $width, $height);
        }

        usort($candidates, fn (array $a, array $b): int => abs($this->area($a) - 16000) <=> abs($this->area($b) - 16000));

        return array_slice($candidates, 0, 12);
    }

    /**
     * @param array<int,array<int,bool>> $visited
     * @return array{bounds:array{0:int,1:int,2:int,3:int},pixels:int}
     */
    private function floodFill(GdImage $image, array &$visited, int $startX, int $startY): array
    {
        $width = imagesx($image);
        $height = imagesy($image);
        $queue = [[$startX, $startY]];
        $visited[$startY][$startX] = true;
        $left = $right = $startX;
        $top = $bottom = $startY;
        $pixels = 0;

        while ($queue !== []) {
            [$x, $y] = array_pop($queue);
            $pixels++;
            $left = min($left, $x);
            $right = max($right, $x);
            $top = min($top, $y);
            $bottom = max($bottom, $y);

            foreach ([[$x + 1, $y], [$x - 1, $y], [$x, $y + 1], [$x, $y - 1]] as [$nx, $ny]) {
                if ($nx < 0 || $ny < 0 || $nx >= $width || $ny >= $height || $visited[$ny][$nx]) {
                    continue;
                }
                $visited[$ny][$nx] = true;
                if ($this->isDarkPixel($image, $nx, $ny)) {
                    $queue[] = [$nx, $ny];
                }
            }
        }

        return ['bounds' => [$left, $top, $right, $bottom], 'pixels' => $pixels];
    }

    private function cropCandidate(GdImage $image, array $bounds): GdImage
    {
        [$left, $top, $right, $bottom] = $bounds;
        $side = max($right - $left + 1, $bottom - $top + 1);
        $candidate = imagecreatetruecolor($side, $side);
        $white = imagecolorallocate($candidate, 255, 255, 255);
        imagefill($candidate, 0, 0, $white);
        imagecopy($candidate, $image, 0, 0, $left, $top, min($side, imagesx($image) - $left), min($side, imagesy($image) - $top));

        return $candidate;
    }

    private function squareBounds(array $bounds, int $maxWidth, int $maxHeight): array
    {
        [$left, $top, $right, $bottom] = $bounds;
        $side = max($right - $left + 1, $bottom - $top + 1);
        $centerX = intdiv($left + $right, 2);
        $centerY = intdiv($top + $bottom, 2);
        $half = intdiv($side, 2);

        return [
            max(0, $centerX - $half),
            max(0, $centerY - $half),
            min($maxWidth - 1, $centerX - $half + $side - 1),
            min($maxHeight - 1, $centerY - $half + $side - 1),
        ];
    }

    private function isDarkPixel(GdImage $image, int $x, int $y): bool
    {
        $rgb = imagecolorsforindex($image, imagecolorat($image, $x, $y));
        $max = max($rgb['red'], $rgb['green'], $rgb['blue']);
        $min = min($rgb['red'], $rgb['green'], $rgb['blue']);
        $luma = (int) round($rgb['red'] * 0.299 + $rgb['green'] * 0.587 + $rgb['blue'] * 0.114);

        return $luma < 120 && ($max - $min) < 80;
    }

    private function clearCenterAvatar(GdImage $image): void
    {
        $side = min(imagesx($image), imagesy($image));
        $clearSide = max(10, (int) round($side * 0.11));
        $start = (int) floor(($side - $clearSide) / 2);
        $end = $start + $clearSide;
        $white = imagecolorallocate($image, 255, 255, 255);
        imagefilledrectangle($image, $start, $start, $end, $end, $white);
    }

    private function area(array $bounds): int
    {
        return max(0, $bounds[2] - $bounds[0] + 1) * max(0, $bounds[3] - $bounds[1] + 1);
    }

    private function intersects(array $a, array $b): bool
    {
        return !($a[2] < $b[0] || $b[2] < $a[0] || $a[3] < $b[1] || $b[3] < $a[1]);
    }
}
