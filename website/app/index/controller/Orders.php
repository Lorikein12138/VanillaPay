<?php
namespace app\index\controller;

use app\common\repository\OrderRepositoryInterface;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Orders
{
    public function __construct(private OrderRepositoryInterface $orders)
    {
    }

    public function index(Request $request)
    {
        $page = max(1, (int) $request->get('page', 1));
        $data = $this->orders->paginateByUser((int) Session::get('user_id'), ['status' => $request->get('status', '')], $page, 30);
        return View::fetch('/orders', ['data' => $data, 'query' => $request->get()]);
    }
}
