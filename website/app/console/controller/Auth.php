<?php
namespace app\console\controller;

use app\common\exception\AuthException;
use app\common\service\AdminAuthService;
use app\common\service\AuditLogger;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Auth
{
    public function __construct(private AdminAuthService $auth, private AuditLogger $audit)
    {
    }

    public function loginForm()
    {
        return View::fetch('/login');
    }

    public function login(Request $request)
    {
        try {
            $admin = $this->auth->login((string) $request->post('username', ''), (string) $request->post('password', ''), $request->ip());
            Session::set('admin_id', $admin['id']);
            $this->audit->login('admin', (int) $admin['id'], $request->ip(), (string) $request->header('user-agent'), 'ok');
            return redirect('/console/dashboard');
        } catch (AuthException $e) {
            $this->audit->login('admin', 0, $request->ip(), (string) $request->header('user-agent'), 'fail');
            Session::flash('flash', $e->getMessage());
            return redirect('/console/login');
        }
    }

    public function logout()
    {
        Session::delete('admin_id');
        return redirect('/console/login');
    }
}
