<?php
namespace app\console\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderExpirationService;
use think\facade\Db;
use think\facade\View;

class Dashboard
{
    public function __construct(private OrderRepositoryInterface $orders, private OrderExpirationService $expiration)
    {
    }

    public function index()
    {
        $this->expiration->refresh();
        $all = $this->orders->paginateAll([], 1, 1);
        $paid = $this->orders->paginateAll(['status' => 'paid'], 1, 1);
        $pending = $this->orders->paginateAll(['status' => 'pending'], 1, 1);
        $expired = $this->orders->paginateAll(['status' => 'expired'], 1, 1);

        return View::fetch('/dashboard', [
            'totalOrders' => $all['total'],
            'paidOrders' => $paid['total'],
            'pendingOrders' => $pending['total'],
            'expiredOrders' => $expired['total'],
            'paidAmount' => $this->sumPaidAmount(),
            'paidAlipayAmount' => $this->sumPaidAmount('alipay'),
            'paidWxpayAmount' => $this->sumPaidAmount('wxpay'),
        ]);
    }

    private function sumPaidAmount(string $channel = ''): string
    {
        $query = Db::name('orders')->where('status', 'paid');
        if ($channel !== '') {
            $query->where('channel', $channel);
        }

        return number_format((float) $query->sum('real_amount'), 2, '.', '');
    }
}
