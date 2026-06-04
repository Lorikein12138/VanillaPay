<?php
namespace app\index\controller;

use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\support\Clock;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Orders
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private AmountLockRepositoryInterface $locks,
        private Clock $clock,
    )
    {
    }

    public function index(Request $request)
    {
        $page = max(1, (int) $request->get('page', 1));
        $query = $request->get();
        $query['status'] = (string) ($query['status'] ?? '');
        $data = $this->orders->paginateByUser((int) Session::get('user_id'), ['status' => $query['status']], $page, 30);
        foreach ($data['items'] as &$order) {
            $order['status_label'] = match ($order['status'] ?? '') {
                'paid' => '已支付',
                'expired' => '已过期',
                default => '待支付',
            };
            $order['status_badge_class'] = match ($order['status'] ?? '') {
                'paid' => 'bg-emerald-50 text-emerald-700 ring-emerald-200',
                'expired' => 'bg-zinc-100 text-zinc-600 ring-zinc-200',
                default => 'bg-amber-50 text-amber-700 ring-amber-200',
            };
        }
        unset($order);
        return View::fetch('/orders', ['data' => $data, 'query' => $query]);
    }

    public function expire()
    {
        $now = $this->clock->now();
        $expired = $this->orders->markExpiredBatch($now);
        $locks = $this->locks->releaseExpired($now);
        $deleted = $this->orders->deleteExpiredByUser((int) Session::get('user_id'));

        Session::flash('flash', "已删除过期订单：{$deleted} 笔，新标记过期 {$expired} 笔，释放金额锁 {$locks} 条。");
        Session::flash('flash_tone', 'success');
        return redirect('/orders');
    }
}
