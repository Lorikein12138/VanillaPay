<?php
use app\common\support\SignHelper;
use PHPUnit\Framework\TestCase;

final class SignHelperTest extends TestCase
{
    public function test_excludes_sign_and_empty_sorts_and_appends_key(): void
    {
        $params = ['pid' => '100', 'money' => '10.00', 'name' => 't', 'type' => 'wxpay', 'out_trade_no' => 'A1', 'notify_url' => 'http://x', 'sign' => 'old', 'sign_type' => 'MD5', 'blank' => ''];
        $expected = md5('money=10.00&name=t&notify_url=http://x&out_trade_no=A1&pid=100&type=wxpay' . 'KEY');
        $this->assertSame($expected, SignHelper::epayStyle($params, 'KEY'));
    }
}
