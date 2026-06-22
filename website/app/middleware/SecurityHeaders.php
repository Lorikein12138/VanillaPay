<?php
namespace app\middleware;

class SecurityHeaders
{
    public function handle($request, \Closure $next)
    {
        $response = $next($request);
        return $response->header([
            'X-Content-Type-Options' => 'nosniff',
            'X-Frame-Options' => 'SAMEORIGIN',
            'Referrer-Policy' => 'strict-origin-when-cross-origin',
            'Strict-Transport-Security' => 'max-age=31536000; includeSubDomains',
        ]);
    }
}
