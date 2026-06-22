<?php
namespace app\common\service;

use app\common\dto\CreateOrderInput;
use app\common\exception\ChannelBusyException;
use app\common\exception\GatewayException;
use app\common\exception\ValidationException;
use app\common\protocol\PayProtocolAdapter;
use app\common\repository\UserRepositoryInterface;

final class GatewayOrderCreator
{
    public function __construct(
        private UserRepositoryInterface $users,
        private OrderCreationService $creation,
    ) {
    }

    public function create(PayProtocolAdapter $adapter, array $params, string $clientIp): array
    {
        $parsed = $adapter->parseOrder($params);
        $merchant = $this->users->findByPid($parsed->pid);
        if (!$merchant || (int) ($merchant['status'] ?? 0) !== 1) {
            throw new GatewayException(-2, '商户不存在或已封禁');
        }
        if (!$adapter->verifyOrderSign($params, (string) $merchant['api_key'])) {
            throw new GatewayException(-1, '签名错误');
        }
        if ($parsed->channel === '') {
            throw new GatewayException(-3, '支付类型不支持');
        }

        try {
            return $this->creation->create(new CreateOrderInput(
                userId: (int) $merchant['id'],
                outTradeNo: $parsed->outTradeNo,
                protocol: $adapter->code(),
                channel: $parsed->channel,
                money: $parsed->money,
                productName: $parsed->productName,
                notifyUrl: $parsed->notifyUrl,
                returnUrl: $parsed->returnUrl,
                param: $parsed->param,
                clientIp: $clientIp,
                floatMode: (string) ($merchant['float_mode'] ?? 'up'),
                floatStep: (string) ($merchant['float_step'] ?? '0.01'),
                floatMax: (string) ($merchant['float_max'] ?? '0.10'),
                timeoutSec: (int) ($merchant['order_timeout'] ?? 300),
            ));
        } catch (ValidationException $e) {
            throw new GatewayException(-4, $e->getMessage());
        } catch (ChannelBusyException $e) {
            throw new GatewayException(-5, $e->getMessage());
        }
    }
}
