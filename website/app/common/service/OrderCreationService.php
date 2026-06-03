<?php
namespace app\common\service;

use app\common\dto\CreateOrderInput;
use app\common\exception\ChannelBusyException;
use app\common\exception\ValidationException;
use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\repository\QrcodeRepositoryInterface;
use app\common\support\Clock;
use app\common\support\Money;

final class OrderCreationService
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private AmountLockRepositoryInterface $locks,
        private QrcodeRepositoryInterface $qrcodes,
        private FloatAmountAllocator $allocator,
        private Clock $clock,
    ) {
    }

    public function create(CreateOrderInput $input): array
    {
        if (!in_array($input->protocol, ['epay', 'codepay', 'yuanpay'], true)) {
            throw new ValidationException('支付协议不支持');
        }
        if (!in_array($input->channel, ['wxpay', 'alipay'], true)) {
            throw new ValidationException('支付渠道不支持');
        }
        if ($input->outTradeNo === '') {
            throw new ValidationException('商户订单号不能为空');
        }
        if ($this->orders->findByUserOutTradeNo($input->userId, $input->outTradeNo)) {
            throw new ValidationException('商户订单号已存在');
        }

        $qrcode = $this->qrcodes->findEnabledByUserChannel($input->userId, $input->channel);
        if (!$qrcode) {
            throw new ValidationException('当前渠道未上传可用收款码');
        }

        $baseCents = Money::toCents($input->money);
        if ($baseCents < 1) {
            throw new ValidationException('金额不合法');
        }

        $expireAt = date('Y-m-d H:i:s', $this->clock->timestamp() + max(60, $input->timeoutSec));
        $candidates = $this->allocator->candidates(
            $baseCents,
            $input->floatMode,
            Money::toCents($input->floatStep),
            Money::toCents($input->floatMax),
        );

        foreach ($candidates as $amountCents) {
            if (!$this->locks->tryAcquire($input->userId, $input->channel, $amountCents, $expireAt)) {
                continue;
            }

            try {
                $id = $this->orders->create([
                    'order_no' => $this->makeOrderNo(),
                    'out_trade_no' => $input->outTradeNo,
                    'user_id' => $input->userId,
                    'protocol' => $input->protocol,
                    'channel' => $input->channel,
                    'product_name' => $input->productName,
                    'money' => Money::fromCents($baseCents),
                    'real_amount' => Money::fromCents($amountCents),
                    'qrcode_id' => (int) $qrcode['id'],
                    'status' => 'pending',
                    'notify_url' => $input->notifyUrl,
                    'return_url' => $input->returnUrl,
                    'param' => $input->param,
                    'client_ip' => $input->clientIp,
                    'device_id' => null,
                    'device_trade_no' => '',
                    'paid_at' => null,
                    'expire_at' => $expireAt,
                    'notify_status' => 0,
                    'notify_count' => 0,
                    'create_time' => $this->clock->now(),
                ]);
                $this->locks->attachOrder($input->userId, $input->channel, $amountCents, $id);
                return $this->orders->findById($id);
            } catch (\Throwable $e) {
                $this->locks->release($input->userId, $input->channel, $amountCents);
                throw $e;
            }
        }

        throw new ChannelBusyException('当前通道繁忙，请稍后重试');
    }

    private function makeOrderNo(): string
    {
        return date('YmdHis') . random_int(100000, 999999);
    }
}
