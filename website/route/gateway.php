<?php
use think\facade\Route;

Route::group(function () {
    Route::any('submit.php', '\app\gateway\controller\Epay@submit');
    Route::any('mapi.php', '\app\gateway\controller\Epay@mapi');
    Route::any('creat_order/', '\app\gateway\controller\Codepay@creatOrder');
    Route::any('yuanpay/submit', '\app\gateway\controller\Yuanpay@submit');
    Route::any('yuanpay/mapi', '\app\gateway\controller\Yuanpay@mapi');
})->middleware(\app\middleware\RateLimit::class, 'gateway', 120, 60);

Route::any('api.php', '\app\gateway\controller\Epay@api');
Route::get('pay/status/<order_no>', '\app\gateway\controller\PayPage@status');
Route::get('pay/success/<order_no>', '\app\gateway\controller\PayPage@success');
Route::get('pay/<order_no>', '\app\gateway\controller\PayPage@show');
