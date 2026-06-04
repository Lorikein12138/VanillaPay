<?php
namespace app\index\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderExpirationService;
use think\facade\Session;
use think\facade\View;

class Dashboard
{
    public function __construct(private OrderRepositoryInterface $orders, private OrderExpirationService $expiration)
    {
    }

    public function index()
    {
        $this->expiration->refresh();
        $userId = (int) Session::get('user_id');
        $all = $this->orders->paginateByUser($userId, [], 1, 1);
        $paid = $this->orders->paginateByUser($userId, ['status' => 'paid'], 1, 1);
        $pending = $this->orders->paginateByUser($userId, ['status' => 'pending'], 1, 1);
        $expired = $this->orders->paginateByUser($userId, ['status' => 'expired'], 1, 1);

        return View::fetch('/dashboard', [
            'totalOrders' => $all['total'],
            'paidOrders' => $paid['total'],
            'pendingOrders' => $pending['total'],
            'expiredOrders' => $expired['total'],
            'paidAmount' => $this->orders->sumByUser($userId, ['status' => 'paid']),
            'paidAlipayAmount' => $this->orders->sumByUser($userId, ['status' => 'paid', 'channel' => 'alipay']),
            'paidWxpayAmount' => $this->orders->sumByUser($userId, ['status' => 'paid', 'channel' => 'wxpay']),
        ]);
    }
}
