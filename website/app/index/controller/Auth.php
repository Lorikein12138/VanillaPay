<?php
namespace app\index\controller;

use app\common\exception\AuthException;
use app\common\exception\ValidationException;
use app\common\repository\UserRepositoryInterface;
use app\common\service\AuthService;
use app\common\service\AuditLogger;
use app\common\service\CredentialGenerator;
use app\common\service\EmailVerificationService;
use app\common\service\PasswordHasher;
use app\common\service\RegistrationService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Auth
{
    private const CODE_COOLDOWN_SECONDS = 60;

    public function __construct(private UserRepositoryInterface $users, private AuditLogger $audit)
    {
    }

    public function registerForm()
    {
        return View::fetch('auth/register', [
            'codeCooldownRemaining' => $this->emailCodeCooldownRemaining('register'),
        ]);
    }

    public function sendRegisterCode(Request $request)
    {
        $email = strtolower(trim((string) $request->post('email', '')));
        try {
            if ($this->users->existsEmail($email)) {
                throw new ValidationException('邮箱已被注册');
            }

            $this->ensureEmailCodeCooldown('register');
            $verification = app(EmailVerificationService::class)->sendCode('注册', $email);
            Session::set('register_email_verification', $verification);
            $this->markEmailCodeSent('register');
            Session::flash('flash', '验证码已发送，请查收邮箱');
            Session::flash('flash_tone', 'success');
        } catch (\Throwable $e) {
            Session::flash('flash', $e instanceof ValidationException ? $e->getMessage() : '验证码发送失败，请检查 SMTP 设置');
            Session::flash('flash_tone', 'error');
        }

        return redirect('/register');
    }

    public function register(Request $request)
    {
        try {
            $email = strtolower(trim((string) $request->post('email', '')));
            $email_code = (string) $request->post('email_code', '');
            $verification = Session::get('register_email_verification');
            if (!app(EmailVerificationService::class)->verify(is_array($verification) ? $verification : null, $email, $email_code)) {
                throw new ValidationException('邮箱验证码不正确或已过期');
            }

            $service = new RegistrationService($this->users, new PasswordHasher(), new CredentialGenerator());
            $service->register(
                $request->post('username', ''),
                $email,
                $request->post('password', '')
            );
            Session::delete('register_email_verification');
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
            $this->rotateSessionId();
            Session::set('user_id', $user['id']);
            $this->audit->login('user', (int) $user['id'], $request->ip(), (string) $request->header('user-agent'), 'ok');
            return redirect('/dashboard');
        } catch (AuthException $e) {
            $this->audit->login('user', 0, $request->ip(), (string) $request->header('user-agent'), 'fail');
            Session::flash('flash', $e->getMessage());
            Session::flash('flash_tone', 'error');
            return redirect('/login');
        }
    }

    public function logout()
    {
        Session::delete('user_id');
        $this->rotateSessionId();
        return redirect('/login');
    }

    public function forgotForm(Request $request)
    {
        return View::fetch('auth/forgot', [
            'email' => $request->get('email', ''),
            'codeCooldownRemaining' => $this->emailCodeCooldownRemaining('reset'),
        ]);
    }

    public function forgot(Request $request)
    {
        $email = strtolower(trim((string) $request->post('email', '')));
        if (!filter_var($email, FILTER_VALIDATE_EMAIL)) {
            Session::flash('flash', '邮箱格式不正确');
            Session::flash('flash_tone', 'error');
            return redirect('/forgot');
        }

        $user = $this->users->findByEmail($email);
        if (!$user) {
            Session::flash('flash', '若邮箱存在，重置验证码已发送');
            Session::flash('flash_tone', 'success');
            return redirect('/forgot');
        }

        try {
            $this->ensureEmailCodeCooldown('reset');
            $verification = app(EmailVerificationService::class)->sendCode('重置密码', $email);
            $verification['user_id'] = (int) $user['id'];
            Session::set('reset_email_verification', $verification);
            $this->markEmailCodeSent('reset');
            Session::flash('flash', '重置验证码已发送，请查收邮箱');
            Session::flash('flash_tone', 'success');
            return redirect('/forgot?email=' . urlencode($email));
        } catch (\Throwable $e) {
            Session::flash('flash', $e instanceof ValidationException ? $e->getMessage() : '验证码发送失败，请检查 SMTP 设置');
            Session::flash('flash_tone', 'error');
            return redirect('/forgot');
        }
    }

    public function resetForm(Request $request)
    {
        return redirect('/forgot?email=' . urlencode((string) $request->get('email', '')));
    }

    public function reset(Request $request)
    {
        $email = strtolower(trim((string) $request->post('email', '')));
        $email_code = (string) $request->post('email_code', '');
        $user = $this->users->findByEmail($email);
        $verification = Session::get('reset_email_verification');
        $record = is_array($verification) ? $verification : null;

        if (!$user || !$record || (int) ($record['user_id'] ?? 0) !== (int) $user['id'] || !app(EmailVerificationService::class)->verify($record, $email, $email_code)) {
            Session::flash('flash', '邮箱验证码不正确或已过期');
            Session::flash('flash_tone', 'error');
            return redirect('/forgot?email=' . urlencode($email));
        }

        $password = $request->post('password', '');
        if (strlen($password) < 8) {
            Session::flash('flash', '密码至少 8 位');
            Session::flash('flash_tone', 'error');
            return redirect('/forgot?email=' . urlencode($email));
        }

        $this->users->update((int) $user['id'], [
            'password_hash' => (new PasswordHasher())->hash($password),
            'login_fail_count' => 0,
            'locked_until' => null,
        ]);
        Session::delete('reset_email_verification');
        Session::flash('flash', '密码已重置，请登录');
        Session::flash('flash_tone', 'success');
        return redirect('/login');
    }

    private function ensureEmailCodeCooldown(string $scene): void
    {
        $key = $scene === 'register' ? 'email_code_sent_at_register' : 'email_code_sent_at_reset';
        $lastSentAt = (int) Session::get($key, 0);
        $remaining = self::CODE_COOLDOWN_SECONDS - (time() - $lastSentAt);
        if ($remaining > 0) {
            throw new ValidationException('验证码发送过于频繁，请 ' . $remaining . ' 秒后重试');
        }
    }

    private function markEmailCodeSent(string $scene): void
    {
        $key = $scene === 'register' ? 'email_code_sent_at_register' : 'email_code_sent_at_reset';
        Session::set($key, time());
    }

    private function emailCodeCooldownRemaining(string $scene): int
    {
        $key = $scene === 'register' ? 'email_code_sent_at_register' : 'email_code_sent_at_reset';
        $lastSentAt = (int) Session::get($key, 0);
        return max(0, self::CODE_COOLDOWN_SECONDS - (time() - $lastSentAt));
    }

    private function rotateSessionId(): void
    {
        if (session_status() === PHP_SESSION_ACTIVE) {
            session_regenerate_id(true);
        }
    }
}
