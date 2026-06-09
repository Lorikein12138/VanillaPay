<?php
namespace app\console\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\service\OrderExpirationService;
use think\facade\View;

class Dashboard
{
    public function __construct(private OrderRepositoryInterface $orders, private OrderExpirationService $expiration)
    {
    }

    public function index()
    {
        $this->expiration->refresh();

        return View::fetch('/dashboard', $this->orders->dashboardMetricsAll());
    }
}
