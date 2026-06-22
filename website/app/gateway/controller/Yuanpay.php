<?php
namespace app\gateway\controller;

use app\common\exception\GatewayException;
use app\common\protocol\YuanpayAdapter;
use app\common\service\GatewayOrderCreator;
use think\Request;

class Yuanpay
{
    public function __construct(private GatewayOrderCreator $creator, private YuanpayAdapter $adapter)
    {
    }

    public function submit(Request $request)
    {
        try {
            $order = $this->creator->create($this->adapter, $request->param(), $request->ip());
            return redirect('/pay/' . $order['order_no']);
        } catch (GatewayException $e) {
            return '下单失败(' . $e->errCode . '): ' . $e->getMessage();
        }
    }

    public function mapi(Request $request)
    {
        try {
            $order = $this->creator->create($this->adapter, $request->param(), $request->ip());
            return json([
                'code' => 1,
                'trade_no' => $order['order_no'],
                'money' => $order['real_amount'],
                'payurl' => (string) url('/pay/' . $order['order_no'], [], false, true),
            ]);
        } catch (GatewayException $e) {
            return json(['code' => $e->errCode, 'msg' => $e->getMessage()]);
        }
    }
}
