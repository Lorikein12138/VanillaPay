<?php
namespace app\console\controller;

use think\facade\Session;

class Index
{
    public function index()
    {
        if (Session::has('admin_id')) {
            return redirect('/console/dashboard');
        }

        return redirect('/console/login');
    }
}
