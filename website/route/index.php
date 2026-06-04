<?php
use think\facade\Route;

Route::get('/', '\app\index\controller\Index@home')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('dashboard', '\app\index\controller\Dashboard@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('credentials', '\app\index\controller\Credentials@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('qrcodes', '\app\index\controller\Qrcodes@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::post('qrcodes/upload', '\app\index\controller\Qrcodes@upload')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::post('qrcodes/delete', '\app\index\controller\Qrcodes@delete')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('devices', '\app\index\controller\Devices@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::post('devices/create', '\app\index\controller\Devices@create')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::post('devices/delete', '\app\index\controller\Devices@delete')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('orders', '\app\index\controller\Orders@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('float', '\app\index\controller\FloatSettings@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::post('float', '\app\index\controller\FloatSettings@save')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('order-test', '\app\index\controller\OrderTest@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::post('order-test', '\app\index\controller\OrderTest@create')->middleware(\app\index\middleware\AuthCheck::class)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('docs', '\app\index\controller\Docs@index')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('register', '\app\index\controller\Auth@registerForm');
Route::post('register', '\app\index\controller\Auth@register')->middleware(\app\middleware\RateLimit::class, 'auth', 20, 60)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('login', '\app\index\controller\Auth@loginForm');
Route::post('login', '\app\index\controller\Auth@login')->middleware(\app\middleware\RateLimit::class, 'auth', 20, 60)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('logout', '\app\index\controller\Auth@logout');
Route::get('forgot', '\app\index\controller\Auth@forgotForm');
Route::post('forgot', '\app\index\controller\Auth@forgot')->middleware(\app\middleware\RateLimit::class, 'auth', 20, 60)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('reset', '\app\index\controller\Auth@resetForm');
Route::post('reset', '\app\index\controller\Auth@reset')->middleware(\app\middleware\RateLimit::class, 'auth', 20, 60)->middleware(\app\middleware\VerifyCsrf::class);
