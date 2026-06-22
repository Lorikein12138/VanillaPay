<?php
namespace app\gateway\controller;

use app\common\exception\GatewayException;
use app\common\protocol\EpayAdapter;
use app\common\repository\AmountLockRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\repository\UserRepositoryInterface;
use app\common\service\GatewayOrderCreator;
use app\common\support\Money;
use think\Request;

class Epay
{
    public function __construct(
        private GatewayOrderCreator $creator,
        private EpayAdapter $adapter,
        private OrderRepositoryInterface $orders,
        private UserRepositoryInterface $users,
        private AmountLockRepositoryInterface $locks,
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
        $act = (string) $request->param('act');
        if ($act === 'server_status') {
            return json([
                'code' => 1,
                'msg' => 'ok',
                'service' => 'VanillaPay',
                'server_time' => date('Y-m-d H:i:s'),
            ]);
        }

        if (!in_array($act, ['order', 'order_status', 'close'], true)) {
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

        if ($act === 'close') {
            if ($order['status'] !== 'pending') {
                return json(['code' => -7, 'msg' => '只有待支付订单可以关闭']);
            }
            if (!$this->orders->closePending((int) $order['id'])) {
                return json(['code' => -7, 'msg' => '只有待支付订单可以关闭']);
            }
            $this->locks->release((int) $merchant['id'], (string) $order['channel'], Money::toCents((string) $order['real_amount']));

            return json([
                'code' => 1,
                'msg' => '订单已关闭',
                'trade_no' => $order['order_no'],
                'out_trade_no' => $order['out_trade_no'],
                'status' => 0,
                'status_text' => 'expired',
            ]);
        }

        if ($act === 'order_status') {
            return json([
                'code' => 1,
                'trade_no' => $order['order_no'],
                'out_trade_no' => $order['out_trade_no'],
                'status' => $order['status'] === 'paid' ? 1 : 0,
                'status_text' => $order['status'],
            ]);
        }

        return json([
            'code' => 1,
            'trade_no' => $order['order_no'],
            'out_trade_no' => $order['out_trade_no'],
            'status' => $order['status'] === 'paid' ? 1 : 0,
            'status_text' => $order['status'],
            'type' => $order['channel'],
            'name' => $order['product_name'],
            'money' => $order['money'],
            'real_money' => $order['real_amount'],
            'create_time' => $order['create_time'],
            'expire_at' => $order['expire_at'],
            'paid_at' => $order['paid_at'] ?? '',
            'param' => $order['param'] ?? '',
            'notify_status' => (int) ($order['notify_status'] ?? 0),
        ]);
    }
}
