<?php
use think\facade\Route;

Route::get('/', '\app\index\controller\Index@home')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('dashboard', '\app\index\controller\Index@home')->middleware(\app\index\middleware\AuthCheck::class);
Route::get('register', '\app\index\controller\Auth@registerForm');
Route::post('register', '\app\index\controller\Auth@register');
Route::get('login', '\app\index\controller\Auth@loginForm');
Route::post('login', '\app\index\controller\Auth@login');
Route::get('logout', '\app\index\controller\Auth@logout');
Route::get('forgot', '\app\index\controller\Auth@forgotForm');
Route::post('forgot', '\app\index\controller\Auth@forgot');
Route::get('reset', '\app\index\controller\Auth@resetForm');
Route::post('reset', '\app\index\controller\Auth@reset');
