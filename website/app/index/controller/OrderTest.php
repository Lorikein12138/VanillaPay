<?php
namespace app\index\controller;

use app\common\dto\CreateOrderInput;
use app\common\repository\UserRepositoryInterface;
use app\common\service\OrderCreationService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class OrderTest
{
    public function __construct(private UserRepositoryInterface $users, private OrderCreationService $orders)
    {
    }

    public function index()
    {
        return View::fetch('/order_test', [
            'user' => $this->users->findById((int) Session::get('user_id')),
            'defaultAmount' => '0.01',
        ]);
    }

    public function create(Request $request)
    {
        $user = $this->users->findById((int) Session::get('user_id'));
        if (!$user) {
            Session::flash('flash', '登录状态已失效，请重新登录');
            Session::flash('flash_tone', 'error');
            return redirect('/login');
        }

        $money = trim((string) $request->post('money', '0.01'));
        if (!preg_match('/^\d+(\.\d{1,2})?$/', $money) || (float) $money <= 0) {
            Session::flash('flash', '测试金额不合法');
            Session::flash('flash_tone', 'error');
            return redirect('/order-test');
        }

        $channel = (string) $request->post('channel', 'wxpay');
        if (!in_array($channel, ['wxpay', 'alipay'], true)) {
            Session::flash('flash', '付款渠道不支持');
            Session::flash('flash_tone', 'error');
            return redirect('/order-test');
        }

        try {
            $order = $this->orders->create(new CreateOrderInput(
                userId: (int) $user['id'],
                outTradeNo: 'TEST' . date('YmdHis') . random_int(100000, 999999),
                protocol: 'epay',
                channel: $channel,
                money: number_format((float) $money, 2, '.', ''),
                productName: 'VanillaPay 订单测试',
                notifyUrl: '',
                returnUrl: rtrim($request->domain(), '/') . '/orders',
                param: 'order-test',
                clientIp: $request->ip(),
                floatMode: (string) ($user['float_mode'] ?? 'up'),
                floatStep: (string) ($user['float_step'] ?? '0.01'),
                floatMax: (string) ($user['float_max'] ?? '0.10'),
                timeoutSec: (int) ($user['order_timeout'] ?? 300),
            ));
        } catch (\Throwable $e) {
            Session::flash('flash', '测试下单失败：' . $e->getMessage());
            Session::flash('flash_tone', 'error');
            return redirect('/order-test');
        }

        Session::flash('flash', '测试订单已创建：' . $order['order_no']);
        Session::flash('flash_tone', 'success');
        return redirect('/pay/' . $order['order_no']);
    }
}
