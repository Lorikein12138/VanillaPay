<?php
namespace app\console\controller;

use think\facade\Db;
use think\facade\View;

class Devices
{
    public function index()
    {
        return View::fetch('/devices', ['list' => Db::name('devices')->order('id', 'desc')->limit(200)->select()->toArray()]);
    }
}
