<?php
namespace app\index\controller;

use app\common\repository\OrderRepositoryInterface;
use app\common\repository\UserRepositoryInterface;
use think\facade\Session;
use think\facade\View;

class Dashboard
{
    public function __construct(private UserRepositoryInterface $users, private OrderRepositoryInterface $orders)
    {
    }

    public function index()
    {
        $user = $this->users->findById((int) Session::get('user_id'));
        $paid = $this->orders->paginateByUser((int) $user['id'], ['status' => 'paid'], 1, 1);
        $all = $this->orders->paginateByUser((int) $user['id'], [], 1, 1);
        return View::fetch('/dashboard', ['user' => $user, 'paidTotal' => $paid['total'], 'allTotal' => $all['total']]);
    }
}
