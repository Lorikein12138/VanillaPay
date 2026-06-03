<?php
namespace app\index\controller;

use app\common\exception\AuthException;
use app\common\exception\ValidationException;
use app\common\repository\UserRepositoryInterface;
use app\common\service\AuthService;
use app\common\service\CredentialGenerator;
use app\common\service\PasswordHasher;
use app\common\service\PasswordResetService;
use app\common\service\RegistrationService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Auth
{
    public function __construct(private UserRepositoryInterface $users)
    {
    }

    public function registerForm()
    {
        return View::fetch('auth/register');
    }

    public function register(Request $request)
    {
        try {
            $service = new RegistrationService($this->users, new PasswordHasher(), new CredentialGenerator());
            $service->register(
                $request->post('username', ''),
                $request->post('email', ''),
                $request->post('password', '')
            );
            Session::flash('flash', '注册成功，请登录');
            Session::flash('flash_tone', 'success');
            return redirect('/login');
        } catch (ValidationException $e) {
            Session::flash('flash', $e->getMessage());
            Session::flash('flash_tone', 'error');
            return redirect('/register');
        }
    }

    public function loginForm()
    {
        return View::fetch('auth/login');
    }

    public function login(Request $request)
    {
        try {
            $auth = new AuthService($this->users, new PasswordHasher());
            $user = $auth->login(
                $request->post('username', ''),
                $request->post('password', ''),
                $request->ip()
            );
            Session::set('user_id', $user['id']);
            return redirect('/dashboard');
        } catch (AuthException $e) {
            Session::flash('flash', $e->getMessage());
            Session::flash('flash_tone', 'error');
            return redirect('/login');
        }
    }

    public function logout()
    {
        Session::delete('user_id');
        return redirect('/login');
    }

    public function forgotForm()
    {
        return View::fetch('auth/forgot');
    }

    public function forgot(Request $request)
    {
        $user = $this->users->findByEmail($request->post('email', ''));
        if ($user) {
            $token = (new PasswordResetService((string) env('APP_KEY', 'vanilla')))->issue((int) $user['id']);
            trace('reset link: /reset?token=' . $token, 'info');
        }

        Session::flash('flash', '若邮箱存在，重置链接已发送');
        Session::flash('flash_tone', 'success');
        return redirect('/forgot');
    }

    public function resetForm(Request $request)
    {
        return View::fetch('auth/reset', ['token' => $request->get('token', '')]);
    }

    public function reset(Request $request)
    {
        $token = $request->post('token', '');
        $uid = (new PasswordResetService((string) env('APP_KEY', 'vanilla')))->verify($token);
        if (!$uid) {
            Session::flash('flash', '链接无效或已过期');
            Session::flash('flash_tone', 'error');
            return redirect('/forgot');
        }

        $password = $request->post('password', '');
        if (strlen($password) < 8) {
            Session::flash('flash', '密码至少 8 位');
            Session::flash('flash_tone', 'error');
            return redirect('/reset?token=' . urlencode($token));
        }

        $this->users->update((int) $uid, [
            'password_hash' => (new PasswordHasher())->hash($password),
            'login_fail_count' => 0,
            'locked_until' => null,
        ]);
        Session::flash('flash', '密码已重置，请登录');
        Session::flash('flash_tone', 'success');
        return redirect('/login');
    }
}
