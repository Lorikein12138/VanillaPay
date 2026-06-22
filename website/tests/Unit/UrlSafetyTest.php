<?php
use app\common\support\UrlSafety;
use PHPUnit\Framework\TestCase;

final class UrlSafetyTest extends TestCase
{
    public function testRejectsUnsupportedSchemesAndPrivateHosts(): void
    {
        $this->expectException(InvalidArgumentException::class);
        UrlSafety::assertPublicHttpUrl('file:///etc/passwd');
    }

    public function testRejectsLoopbackLiteral(): void
    {
        $this->expectException(InvalidArgumentException::class);
        UrlSafety::assertPublicHttpTarget('http://127.0.0.1/callback');
    }

    public function testRejectsTargetsThatCannotBeResolved(): void
    {
        $this->expectException(InvalidArgumentException::class);
        UrlSafety::assertPublicHttpTarget('http://vanillapay-unresolvable.invalid/callback');
    }
}
