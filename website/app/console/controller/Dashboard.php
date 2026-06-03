<?php
namespace app\console\controller;

use app\common\repository\OrderRepositoryInterface;
use think\facade\View;

class Dashboard
{
    public function __construct(private OrderRepositoryInterface $orders)
    {
    }

    public function index()
    {
        $paid = $this->orders->paginateAll(['status' => 'paid'], 1, 1);
        $all = $this->orders->paginateAll([], 1, 1);
        return View::fetch('/dashboard', ['paidTotal' => $paid['total'], 'allTotal' => $all['total']]);
    }
}
