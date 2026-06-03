<?php
namespace app\index\controller;

use app\common\repository\UserRepositoryInterface;
use think\facade\Session;

class Index
{
    public function __construct(private UserRepositoryInterface $users)
    {
    }

    public function home()
    {
        $user = $this->users->findById((int) Session::get('user_id'));
        if (!$user) {
            Session::delete('user_id');
            return redirect('/login');
        }

        return redirect('/dashboard');
    }
}
