<?php
namespace app\device\controller;

use app\common\dto\PushInput;
use app\common\repository\DeviceRepositoryInterface;
use app\common\repository\RiskEventRepositoryInterface;
use app\common\service\DeviceSigner;
use app\common\service\PaymentMatcher;
use app\common\service\ReplayGuard;
use app\common\support\Clock;
use app\common\support\Money;
use think\Request;

class Push
{
    public function __construct(
        private DeviceRepositoryInterface $devices,
        private DeviceSigner $signer,
        private PaymentMatcher $matcher,
        private RiskEventRepositoryInterface $risks,
        private ReplayGuard $replay,
        private Clock $clock,
    ) {
    }

    public function report(Request $request)
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
        if (!$this->replay->firstUse($device['id'] . ':' . ($params['sign'] ?? ''), 300)) {
            return json(['code' => 1, 'matched' => false, 'msg' => 'duplicate']);
        }

        $push = new PushInput(
            userId: (int) $device['user_id'],
            deviceId: (int) $device['id'],
            channel: (string) ($params['channel'] ?? ''),
            amountCents: Money::toCents($params['price'] ?? '0'),
            tradeNoDevice: (string) ($params['trade_no_device'] ?? ''),
        );
        $result = $this->matcher->match($push);

        if ($result->isUnmatched()) {
            $this->risks->create([
                'type' => 'unmatched_payment',
                'user_id' => (int) $device['user_id'],
                'device_id' => (int) $device['id'],
                'level' => 'warning',
                'detail' => json_encode([
                    'channel' => $push->channel,
                    'price' => $params['price'] ?? null,
                    'trade_no_device' => $push->tradeNoDevice,
                    'raw' => $params['raw'] ?? null,
                ], JSON_UNESCAPED_UNICODE),
            ]);
        }

        return json([
            'code' => 1,
            'matched' => $result->isMatched(),
            'order_no' => $result->order['order_no'] ?? null,
        ]);
    }
}
