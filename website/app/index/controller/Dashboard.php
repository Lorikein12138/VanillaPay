<?php
namespace app\index\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderExpirationService;
use think\facade\Session;
use think\facade\View;

class Dashboard
{
    public function __construct(private OrderRepositoryInterface $orders, private OrderExpirationService $expiration)
    {
    }

    public function index()
    {
        $this->expiration->refresh();
        $userId = (int) Session::get('user_id');

        return View::fetch('/dashboard', $this->orders->dashboardMetricsByUser($userId));
    }
}
