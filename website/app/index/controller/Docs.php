<?php
namespace app\index\controller;

use app\common\repository\UserRepositoryInterface;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Docs
{
    public function __construct(private UserRepositoryInterface $users)
    {
    }

    public function index(Request $request)
    {
        return View::fetch('/docs', ['user' => $this->users->findById((int) Session::get('user_id')), 'baseUrl' => $request->domain()]);
    }
}
