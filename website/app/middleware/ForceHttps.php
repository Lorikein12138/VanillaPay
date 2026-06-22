<?php
namespace app\middleware;

class ForceHttps
{
    public function handle($request, \Closure $next)
    {
        if (!$request->isSsl() && env('APP_ENV', 'local') === 'production') {
            return redirect('https://' . $request->host() . $request->url());
        }
        return $next($request);
    }
}
