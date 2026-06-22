<?php
namespace app\console\controller;

use app\common\repository\OrderRepositoryInterface;
use think\Request;
use think\facade\View;

class Orders
{
    public function __construct(private OrderRepositoryInterface $orders)
    {
    }

    public function index(Request $request)
    {
        $page = max(1, (int) $request->get('page', 1));
        return View::fetch('/orders', [
            'data' => $this->orders->paginateAll($request->get(), $page, 30),
            'query' => $request->get(),
        ]);
    }
}
