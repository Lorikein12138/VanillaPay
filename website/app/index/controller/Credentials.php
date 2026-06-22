<?php
namespace app\index\controller;

use app\common\repository\UserRepositoryInterface;
use think\facade\Session;
use think\facade\View;

class Credentials
{
    public function __construct(private UserRepositoryInterface $users)
    {
    }

    public function index()
    {
        return View::fetch('/credentials', ['user' => $this->users->findById((int) Session::get('user_id'))]);
    }
}
