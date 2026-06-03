<?php
namespace app\gateway\controller;

use app\common\exception\GatewayException;
use app\common\protocol\EpayAdapter;
use app\common\repository\OrderRepositoryInterface;
use app\common\repository\UserRepositoryInterface;
use app\common\service\GatewayOrderCreator;
use think\Request;

class Epay
{
    public function __construct(
        private GatewayOrderCreator $creator,
        private EpayAdapter $adapter,
        private OrderRepositoryInterface $orders,
        private UserRepositoryInterface $users,
    ) {
    }

    public function submit(Request $request)
    {
        try {
            $order = $this->creator->create($this->adapter, $request->param(), $request->ip());
            return redirect('/pay/' . $order['order_no']);
        } catch (GatewayException $e) {
            return '支付下单失败(' . $e->errCode . '): ' . $e->getMessage();
        }
    }

    public function mapi(Request $request)
    {
        try {
            $order = $this->creator->create($this->adapter, $request->param(), $request->ip());
            return json([
                'code' => 1,
                'trade_no' => $order['order_no'],
                'out_trade_no' => $order['out_trade_no'],
                'money' => $order['real_amount'],
                'payurl' => (string) url('/pay/' . $order['order_no'], [], false, true),
            ]);
        } catch (GatewayException $e) {
            return json(['code' => $e->errCode, 'msg' => $e->getMessage()]);
        }
    }

    public function api(Request $request)
    {
        if ($request->param('act') !== 'order') {
            return json(['code' => -3, 'msg' => '不支持的 act']);
        }
        $merchant = $this->users->findByPid((string) $request->param('pid'));
        if (!$merchant || !hash_equals((string) $merchant['api_key'], (string) $request->param('key'))) {
            return json(['code' => -1, 'msg' => '商户校验失败']);
        }
        $order = $this->orders->findByUserOutTradeNo((int) $merchant['id'], (string) $request->param('out_trade_no'));
        if (!$order) {
            return json(['code' => -6, 'msg' => '订单不存在']);
        }
        return json([
            'code' => 1,
            'trade_no' => $order['order_no'],
            'out_trade_no' => $order['out_trade_no'],
            'status' => $order['status'] === 'paid' ? 1 : 0,
            'money' => $order['real_amount'],
        ]);
    }
}
