<?php
namespace app\common\protocol;

use app\common\dto\ParsedOrder;
use app\common\support\SignHelper;

class EpayAdapter implements PayProtocolAdapter
{
    protected const TYPE_TO_CHANNEL = ['alipay' => 'alipay', 'wxpay' => 'wxpay'];
    protected const CHANNEL_TO_TYPE = ['alipay' => 'alipay', 'wxpay' => 'wxpay'];

    public function code(): string
    {
        return 'epay';
    }

    public function verifyOrderSign(array $params, string $apiKey): bool
    {
        $sign = (string) ($params['sign'] ?? '');
        return $sign !== '' && hash_equals(SignHelper::epayStyle($params, $apiKey), $sign);
    }

    public function parseOrder(array $params): ParsedOrder
    {
        $type = (string) ($params['type'] ?? '');
        return new ParsedOrder(
            pid: (string) ($params['pid'] ?? ''),
            outTradeNo: (string) ($params['out_trade_no'] ?? ''),
            channel: static::TYPE_TO_CHANNEL[$type] ?? '',
            money: (string) ($params['money'] ?? '0'),
            productName: (string) ($params['name'] ?? ''),
            notifyUrl: (string) ($params['notify_url'] ?? ''),
            returnUrl: (string) ($params['return_url'] ?? ''),
            param: (string) ($params['param'] ?? ''),
        );
    }

    public function buildNotifyParams(array $order, string $pid, string $apiKey): array
    {
        $params = [
            'pid' => $pid,
            'trade_no' => $order['order_no'],
            'out_trade_no' => $order['out_trade_no'],
            'type' => static::CHANNEL_TO_TYPE[$order['channel']] ?? $order['channel'],
            'name' => $order['product_name'],
            'money' => $order['real_amount'],
            'trade_status' => 'TRADE_SUCCESS',
        ];
        $params['sign'] = SignHelper::epayStyle($params, $apiKey);
        $params['sign_type'] = 'MD5';
        return $params;
    }

    public function buildReturnParams(array $order, string $pid, string $apiKey): array
    {
        return $this->buildNotifyParams($order, $pid, $apiKey);
    }

    public function successText(): string
    {
        return 'success';
    }
}
