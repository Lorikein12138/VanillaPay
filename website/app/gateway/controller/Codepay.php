<?php
namespace app\gateway\controller;

use app\common\exception\GatewayException;
use app\common\protocol\CodepayAdapter;
use app\common\service\GatewayOrderCreator;
use think\Request;

class Codepay
{
    public function __construct(private GatewayOrderCreator $creator, private CodepayAdapter $adapter)
    {
    }

    public function creatOrder(Request $request)
    {
        try {
            $order = $this->creator->create($this->adapter, $request->param(), $request->ip());
            return redirect('/pay/' . $order['order_no']);
        } catch (GatewayException $e) {
            return '下单失败(' . $e->errCode . '): ' . $e->getMessage();
        }
    }
}
