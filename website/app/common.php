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

if (!function_exists('asset_version')) {
    function asset_version(string $path): string
    {
        $relativePath = ltrim($path, '/');
        if (str_starts_with($relativePath, 'static/')) {
            $relativePath = 'public/' . $relativePath;
        }

        $file = app()->getRootPath() . str_replace('/', DIRECTORY_SEPARATOR, $relativePath);

        return is_file($file) ? (string) filemtime($file) : '1';
    }
}
