<?php
require __DIR__ . '/index.php';
require __DIR__ . '/gateway.php';

use think\facade\Route;

Route::post('app/heart', '\app\device\controller\Heart@beat')->middleware(\app\middleware\RateLimit::class, 'device', 240, 60);
Route::post('app/push', '\app\device\controller\Push@report')->middleware(\app\middleware\RateLimit::class, 'device', 240, 60);
Route::get('app/config', '\app\device\controller\Config@rules')->middleware(\app\middleware\RateLimit::class, 'device', 240, 60);
