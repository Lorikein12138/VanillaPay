<?php
namespace app\console\middleware;

use think\facade\Session;

class AdminCheck
{
    public function handle($request, \Closure $next)
    {
        if (!Session::has('admin_id')) {
            return redirect('/console/login');
        }
        return $next($request);
    }
}
