<?php
namespace app\common\protocol;

use app\common\dto\ParsedOrder;

final class CodepayAdapter implements PayProtocolAdapter
{
    private const TYPE_TO_CHANNEL = ['1' => 'wxpay', '2' => 'alipay'];
    private const CHANNEL_TO_TYPE = ['wxpay' => '1', 'alipay' => '2'];

    public function code(): string
    {
        return 'codepay';
    }

    public function verifyOrderSign(array $params, string $apiKey): bool
    {
        $sign = (string) ($params['sign'] ?? '');
        $expected = md5((string) ($params['pay_id'] ?? '') . (string) ($params['param'] ?? '')
            . (string) ($params['type'] ?? '') . (string) ($params['price'] ?? '') . $apiKey);
        return $sign !== '' && hash_equals($expected, $sign);
    }

    public function parseOrder(array $params): ParsedOrder
    {
        $type = (string) ($params['type'] ?? '');
        return new ParsedOrder(
            pid: (string) ($params['id'] ?? ''),
            outTradeNo: (string) ($params['pay_id'] ?? ''),
            channel: self::TYPE_TO_CHANNEL[$type] ?? '',
            money: (string) ($params['price'] ?? '0'),
            productName: (string) ($params['name'] ?? ''),
            notifyUrl: (string) ($params['notify_url'] ?? ''),
            returnUrl: (string) ($params['return_url'] ?? ''),
            param: (string) ($params['param'] ?? ''),
        );
    }

    public function buildNotifyParams(array $order, string $pid, string $apiKey): array
    {
        $type = self::CHANNEL_TO_TYPE[$order['channel']] ?? '';
        $params = [
            'id' => $pid,
            'pay_id' => $order['out_trade_no'],
            'param' => $order['param'] ?? '',
            'type' => $type,
            'price' => $order['money'],
            'reallyprice' => $order['real_amount'],
        ];
        $params['sign'] = md5($params['pay_id'] . $params['param'] . $params['type'] . $params['price'] . $params['reallyprice'] . $apiKey);
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
