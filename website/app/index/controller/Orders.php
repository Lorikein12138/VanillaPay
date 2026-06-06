<?php
namespace app\index\controller;

use app\common\exception\ValidationException;
use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderExpirationService;
use app\common\service\OrderSupplementService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Orders
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private OrderExpirationService $expiration,
        private OrderSupplementService $supplements,
    )
    {
    }

    public function index(Request $request)
    {
        $this->expiration->refresh();
        $page = max(1, (int) $request->get('page', 1));
        $query = $request->get();
        $query['status'] = (string) ($query['status'] ?? '');
        $data = $this->orders->paginateByUser((int) Session::get('user_id'), ['status' => $query['status']], $page, 30);
        foreach ($data['items'] as &$order) {
            $order['status_label'] = match ($order['status'] ?? '') {
                'paid' => '已支付',
                'expired' => '已过期',
                default => '待付款',
            };
            $order['status_badge_class'] = match ($order['status'] ?? '') {
                'paid' => 'bg-emerald-50 text-emerald-700 ring-emerald-200',
                'expired' => 'bg-zinc-100 text-zinc-600 ring-zinc-200',
                default => 'bg-amber-50 text-amber-700 ring-amber-200',
            };
            $order['can_supplement'] = in_array((string) ($order['status'] ?? ''), ['pending', 'expired'], true);
        }
        unset($order);
        return View::fetch('/orders', ['data' => $data, 'query' => $query]);
    }

    public function supplement(Request $request)
    {
        try {
            $this->expiration->refresh();
            $this->supplements->supplement((int) Session::get('user_id'), (int) $request->post('id'));
            Session::flash('flash', '补单完成，已触发下游通知。');
            Session::flash('flash_tone', 'success');
        } catch (ValidationException $e) {
            Session::flash('flash', '补单失败：' . $e->getMessage());
            Session::flash('flash_tone', 'error');
        }

        return redirect('/orders');
    }

    public function expire()
    {
        $refreshed = $this->expiration->refresh();
        $deleted = $this->orders->deleteExpiredByUser((int) Session::get('user_id'));

        Session::flash('flash', "已删除过期订单：{$deleted} 笔，新标记过期 {$refreshed['orders']} 笔，释放金额锁 {$refreshed['locks']} 条。");
        Session::flash('flash_tone', 'success');
        return redirect('/orders');
    }
}
