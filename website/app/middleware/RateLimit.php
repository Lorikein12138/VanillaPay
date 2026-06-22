<?php
namespace app\middleware;

use app\common\service\RateLimiter;

class RateLimit
{
    public function __construct(private RateLimiter $limiter)
    {
    }

    public function handle($request, \Closure $next, string $bucket = 'default', int $max = 60, int $window = 60)
    {
        if (!$this->limiter->allow($bucket . ':' . $request->ip(), $max, $window)) {
            return json(['code' => -429, 'msg' => '请求过于频繁，请稍后再试'])->code(429);
        }
        return $next($request);
    }
}
