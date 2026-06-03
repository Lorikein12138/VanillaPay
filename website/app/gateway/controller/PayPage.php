<?php
namespace app\gateway\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\repository\QrcodeRepositoryInterface;
use think\facade\View;

class PayPage
{
    public function __construct(private OrderRepositoryInterface $orders, private QrcodeRepositoryInterface $qrcodes)
    {
    }

    public function show(string $order_no)
    {
        $order = $this->orders->findByOrderNo($order_no);
        if (!$order) {
            return '订单不存在';
        }
        $qr = $this->qrcodes->findById((int) $order['qrcode_id']);
        return View::fetch(app()->getRootPath() . 'view/gateway/pay.html', [
            'order' => $order,
            'channelName' => $order['channel'] === 'wxpay' ? '微信' : '支付宝',
            'qrImage' => $qr['qr_image_path'] ?? '',
        ]);
    }

    public function status(string $order_no)
    {
        $order = $this->orders->findByOrderNo($order_no);
        return json(['paid' => $order && $order['status'] === 'paid', 'return_url' => $order['return_url'] ?? '']);
    }
}
