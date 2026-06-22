<?php

use PHPUnit\Framework\TestCase;

final class CssAssetVersionTest extends TestCase
{
    public function testRenderedPagesUseCacheBustedCssAsset(): void
    {
        $root = dirname(__DIR__, 2);

        foreach ([
            'view/index/layout.html',
            'view/index/merchant_layout.html',
            'view/console/layout.html',
            'view/console/login.html',
            'view/gateway/pay.html',
            'view/gateway/success.html',
        ] as $template) {
            $content = file_get_contents($root . '/' . $template) ?: '';

            $this->assertStringContainsString(
                '/static/dist/app.css?v={:asset_version(\'/static/dist/app.css\')}',
                $content,
                $template . ' must bust cached Tailwind CSS after deployment'
            );
        }
    }
}
