<?php
namespace app\middleware;

use think\facade\Session;

class VerifyCsrf
{
    public function handle($request, \Closure $next)
    {
        if ($request->isPost()) {
            $token = (string) $request->post('_csrf', '');
            $sessionToken = (string) Session::get('_csrf_token', '');
            if ($token === '' || $sessionToken === '' || !hash_equals($sessionToken, $token)) {
                return response('CSRF token invalid', 419);
            }
        }
        return $next($request);
    }
}
