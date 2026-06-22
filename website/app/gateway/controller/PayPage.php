<?php
namespace app\gateway\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\repository\QrcodeRepositoryInterface;
use app\common\service\OrderExpirationService;
use think\facade\View;

class PayPage
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private QrcodeRepositoryInterface $qrcodes,
        private OrderExpirationService $expiration,
    )
    {
    }

    public function show(string $order_no)
    {
        $this->expiration->refresh();
        $order = $this->orders->findByOrderNo($order_no);
        if (!$order) {
            return '订单不存在';
        }
        if (($order['status'] ?? '') === 'paid') {
            return redirect('/pay/success/' . $order_no);
        }
        if (($order['status'] ?? '') === 'expired') {
            return redirect('/pay/expired/' . $order_no);
        }
        $qr = $this->qrcodes->findById((int) $order['qrcode_id']);
        $expireAt = (string) ($order['expire_at'] ?? '');
        $expireTimestamp = $expireAt !== '' ? strtotime($expireAt) : false;
        $remainingSeconds = $expireTimestamp ? max(0, $expireTimestamp - time()) : 0;
        if ($remainingSeconds <= 0) {
            return redirect('/pay/expired/' . $order_no);
        }

        return View::fetch(app()->getRootPath() . 'view/gateway/pay.html', [
            'order' => $order,
            'channelName' => $this->channelName((string) $order['channel']),
            'theme' => $this->channelTheme((string) $order['channel']),
            'qrImage' => $qr['qr_image_path'] ?? '',
            'expireAt' => $expireAt,
            'remainingSeconds' => $remainingSeconds,
            'expiredUrl' => '/pay/expired/' . $order_no,
        ]);
    }

    public function success(string $order_no)
    {
        $order = $this->orders->findByOrderNo($order_no);
        if (!$order) {
            return '订单不存在';
        }
        if (($order['status'] ?? '') !== 'paid') {
            return redirect('/pay/' . $order_no);
        }

        return View::fetch(app()->getRootPath() . 'view/gateway/success.html', [
            'order' => $order,
            'channelName' => $this->channelName((string) $order['channel']),
            'theme' => $this->channelTheme((string) $order['channel']),
            'paidAt' => (string) ($order['paid_at'] ?? '') ?: '-',
            'returnUrl' => (string) ($order['return_url'] ?? '') ?: '/',
        ]);
    }

    public function expired(string $order_no)
    {
        $this->expiration->refresh();
        $order = $this->orders->findByOrderNo($order_no);
        if (!$order) {
            return '订单不存在';
        }
        if (($order['status'] ?? '') === 'paid') {
            return redirect('/pay/success/' . $order_no);
        }

        return View::fetch(app()->getRootPath() . 'view/gateway/expired.html', [
            'order' => $order,
            'channelName' => $this->channelName((string) $order['channel']),
            'theme' => $this->channelTheme((string) $order['channel']),
            'returnUrl' => (string) ($order['return_url'] ?? '') ?: '/',
        ]);
    }

    public function status(string $order_no)
    {
        $this->expiration->refresh();
        $order = $this->orders->findByOrderNo($order_no);
        $expired = $order && ($order['status'] ?? '') === 'expired';

        return json([
            'paid' => $order && $order['status'] === 'paid',
            'expired' => $expired,
            'return_url' => $order['return_url'] ?? '',
            'success_url' => $order ? '/pay/success/' . $order['order_no'] : '',
            'expired_url' => $order ? '/pay/expired/' . $order['order_no'] : '',
        ]);
    }

    private function channelName(string $channel): string
    {
        return $channel === 'wxpay' ? '微信' : '支付宝';
    }

    private function channelTheme(string $channel): array
    {
        return $channel === 'wxpay'
            ? ['code' => 'wxpay', 'accent' => 'emerald']
            : ['code' => 'alipay', 'accent' => 'sky'];
    }
}
