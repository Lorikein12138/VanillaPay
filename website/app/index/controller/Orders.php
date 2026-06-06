<?php
namespace app\index\controller;

use app\common\exception\ValidationException;
use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderDeleteService;
use app\common\service\OrderExpirationService;
use app\common\service\OrderSupplementService;
use think\Request;
use think\facade\Log;
use think\facade\Session;
use think\facade\View;

class Orders
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private OrderExpirationService $expiration,
        private OrderSupplementService $supplements,
        private OrderDeleteService $deletion,
    )
    {
    }

    public function index(Request $request)
    {
        $this->expiration->refresh();
        $userId = (int) Session::get('user_id');
        $page = max(1, (int) $request->get('page', 1));
        $filters = $this->filtersFromArray($request->get());
        $data = $this->orders->paginateByUser((int) Session::get('user_id'), $filters, $page, 10);
        $totalPages = max(1, (int) ceil(((int) $data['total']) / 10));
        if ($page > $totalPages) {
            $page = $totalPages;
            $data = $this->orders->paginateByUser($userId, $filters, $page, 10);
        }

        $this->decorateOrders($data['items']);
        $data['page'] = $page;

        return View::fetch('/orders', [
            'data' => $data,
            'query' => $filters,
            'pagination' => $this->pagination($filters, $page, (int) $data['total'], 10),
        ]);
    }

    public function supplement(Request $request)
    {
        try {
            $this->expiration->refresh();
            $result = $this->supplements->supplement((int) Session::get('user_id'), (int) $request->post('id'));
            if (!($result['callback_dispatched'] ?? true)) {
                Log::error('order supplement callback dispatch failed: order_id=' . (int) $request->post('id') . ' error=' . ($result['callback_error'] ?? 'unknown'));
                Session::flash('flash', '补单完成，但下游通知触发异常，系统已记录，请检查通知状态或等待重试。');
                Session::flash('flash_tone', 'error');
                return $this->redirectToOrders($request);
            }

            Session::flash('flash', '补单完成，已触发下游通知。');
            Session::flash('flash_tone', 'success');
        } catch (ValidationException $e) {
            Session::flash('flash', '补单失败：' . $e->getMessage());
            Session::flash('flash_tone', 'error');
        } catch (\Throwable $e) {
            Log::error('order supplement failed: ' . $e->getMessage());
            Session::flash('flash', '补单失败：系统异常，请稍后重试。');
            Session::flash('flash_tone', 'error');
        }

        return $this->redirectToOrders($request);
    }

    public function supplementRedirect()
    {
        return redirect('/orders')->code(303);
    }

    public function delete(Request $request)
    {
        try {
            $this->expiration->refresh();
            $this->deletion->deleteForUser((int) Session::get('user_id'), (int) $request->post('id'));
            Session::flash('flash', '订单已删除。');
            Session::flash('flash_tone', 'success');
        } catch (ValidationException $e) {
            Session::flash('flash', '删除失败：' . $e->getMessage());
            Session::flash('flash_tone', 'error');
        } catch (\Throwable $e) {
            Log::error('order delete failed: ' . $e->getMessage());
            Session::flash('flash', '删除失败：系统异常，请稍后重试。');
            Session::flash('flash_tone', 'error');
        }

        return $this->redirectToOrders($request);
    }

    public function expire()
    {
        $refreshed = $this->expiration->refresh();
        $deleted = $this->orders->deleteExpiredByUser((int) Session::get('user_id'));

        Session::flash('flash', "已删除过期订单：{$deleted} 笔，新标记过期 {$refreshed['orders']} 笔，释放金额锁 {$refreshed['locks']} 条。");
        Session::flash('flash_tone', 'success');
        return redirect('/orders');
    }

    private function filtersFromArray(array $input): array
    {
        $channel = trim((string) ($input['channel'] ?? ''));
        $status = trim((string) ($input['status'] ?? ''));

        return [
            'order_no' => trim((string) ($input['order_no'] ?? '')),
            'out_trade_no' => trim((string) ($input['out_trade_no'] ?? '')),
            'channel' => in_array($channel, ['wxpay', 'alipay'], true) ? $channel : '',
            'status' => in_array($status, ['pending', 'paid', 'expired'], true) ? $status : '',
        ];
    }

    private function decorateOrders(array &$items): void
    {
        foreach ($items as &$order) {
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
    }

    private function pagination(array $filters, int $page, int $total, int $pageSize): array
    {
        $totalPages = max(1, (int) ceil($total / $pageSize));

        return [
            'page' => $page,
            'total_pages' => $totalPages,
            'has_prev' => $page > 1,
            'has_next' => $page < $totalPages,
            'prev_url' => $this->ordersUrl($filters, max(1, $page - 1)),
            'next_url' => $this->ordersUrl($filters, min($totalPages, $page + 1)),
        ];
    }

    private function redirectToOrders(Request $request)
    {
        return redirect($this->ordersUrl(
            $this->filtersFromArray($request->post()),
            max(1, (int) $request->post('page', 1))
        ))->code(303);
    }

    private function ordersUrl(array $filters, int $page): string
    {
        $query = array_filter($filters, fn ($value): bool => $value !== '' && $value !== null);
        if ($page > 1) {
            $query['page'] = $page;
        }

        return '/orders' . ($query === [] ? '' : '?' . http_build_query($query));
    }
}
