<?php
namespace app\index\middleware;

use think\facade\Session;

class AuthCheck
{
    public function handle($request, \Closure $next)
    {
        if (!Session::has('user_id')) {
            return redirect('/login');
        }

        return $next($request);
    }
}
