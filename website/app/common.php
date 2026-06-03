<?php
// 应用公共文件

use think\facade\Session;

if (!function_exists('csrf_token')) {
    function csrf_token(): string
    {
        if (!Session::has('_csrf_token')) {
            Session::set('_csrf_token', bin2hex(random_bytes(16)));
        }
        return (string) Session::get('_csrf_token');
    }
}
