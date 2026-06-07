<?php
use app\console\middleware\AdminCheck;
use think\facade\Route;

Route::get('', '\app\console\controller\Index@index');
Route::get('login', '\app\console\controller\Auth@loginForm');
Route::post('login', '\app\console\controller\Auth@login')->middleware(\app\middleware\RateLimit::class, 'console_login', 10, 60)->middleware(\app\middleware\VerifyCsrf::class);
Route::get('logout', '\app\console\controller\Auth@logout');

Route::group(function () {
    Route::get('dashboard', '\app\console\controller\Dashboard@index');
    Route::get('merchants', '\app\console\controller\Merchants@index');
    Route::post('merchants/setStatus', '\app\console\controller\Merchants@setStatus')->middleware(\app\middleware\VerifyCsrf::class);
    Route::post('merchants/resetKey', '\app\console\controller\Merchants@resetKey')->middleware(\app\middleware\VerifyCsrf::class);
    Route::post('merchants/saveFloat', '\app\console\controller\Merchants@saveFloat')->middleware(\app\middleware\VerifyCsrf::class);
    Route::get('orders', '\app\console\controller\Orders@index');
    Route::get('devices', '\app\console\controller\Devices@index');
    Route::get('channels', '\app\console\controller\Channels@index');
    Route::post('channels/save', '\app\console\controller\Channels@save')->middleware(\app\middleware\VerifyCsrf::class);
    Route::get('settings', '\app\console\controller\Settings@index');
    Route::post('settings/save', '\app\console\controller\Settings@save')->middleware(\app\middleware\VerifyCsrf::class);
})->middleware(AdminCheck::class);
