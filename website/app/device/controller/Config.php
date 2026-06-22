<?php
namespace app\device\controller;

use app\common\repository\DeviceRepositoryInterface;
use app\common\service\DeviceSigner;
use app\common\support\Clock;
use think\Request;

class Config
{
    public function __construct(private DeviceRepositoryInterface $devices, private DeviceSigner $signer, private Clock $clock)
    {
    }

    public function rules(Request $request)
    {
        $params = $request->param();
        $device = $this->devices->findById((int) ($params['device_id'] ?? 0));
        if (!$device) {
            return json(['code' => -2, 'msg' => '设备不存在']);
        }
        if (!$this->signer->timestampValid((int) ($params['t'] ?? 0), $this->clock->timestamp())) {
            return json(['code' => -7, 'msg' => '时间戳过期']);
        }
        if (!$this->signer->verify($params, (string) $device['device_key'])) {
            return json(['code' => -1, 'msg' => '签名错误']);
        }

        // Incoming-money keywords only (收款/到账/入账/收钱); excludes 付款/退款 so outgoing
        // or refunded payments are never matched. Amount sits right after a keyword, tolerating
        // a few separators (¥ : 「金额」), thousands separators and up to two decimals.
        $keyword = '收款|到账|入账|收钱';
        $amountRegex = '(?:收款|到账|入账|收钱)[^0-9]{0,4}([0-9][0-9,]*(?:\\.[0-9]{1,2})?)';

        return json([
            'code' => 1,
            'version' => 2,
            'rules' => [
                [
                    'channel' => 'wxpay',
                    'package' => 'com.tencent.mm',
                    'keyword' => $keyword,
                    'amountRegex' => $amountRegex,
                ],
                [
                    'channel' => 'alipay',
                    'package' => 'com.eg.android.AlipayGphone',
                    'keyword' => $keyword,
                    'amountRegex' => $amountRegex,
                ],
            ],
        ]);
    }
}
