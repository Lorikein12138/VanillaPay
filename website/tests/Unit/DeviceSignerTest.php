<?php
use app\common\service\DeviceSigner;
use PHPUnit\Framework\TestCase;

final class DeviceSignerTest extends TestCase
{
    public function test_sign_is_deterministic_and_order_independent(): void
    {
        $signer = new DeviceSigner();
        $a = $signer->sign(['device_id' => '5', 'channel' => 'wxpay', 'price' => '10.00', 't' => '100'], 'KEY');
        $b = $signer->sign(['t' => '100', 'price' => '10.00', 'device_id' => '5', 'channel' => 'wxpay'], 'KEY');

        $this->assertSame($a, $b);
        $this->assertSame(hash_hmac('sha256', 'channel=wxpay&device_id=5&price=10.00&t=100', 'KEY'), $a);
        $this->assertMatchesRegularExpression('/^[a-f0-9]{64}$/', $a);
    }

    public function test_verify_accepts_valid_and_rejects_tampered(): void
    {
        $signer = new DeviceSigner();
        $params = ['device_id' => '5', 'channel' => 'wxpay', 'price' => '10.00', 't' => '100'];
        $params['sign'] = $signer->sign($params, 'KEY');

        $this->assertTrue($signer->verify($params, 'KEY'));
        $params['price'] = '99.99';
        $this->assertFalse($signer->verify($params, 'KEY'));
    }
}
