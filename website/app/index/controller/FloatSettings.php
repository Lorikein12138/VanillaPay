<?php
namespace app\index\controller;

use app\common\exception\ValidationException;
use app\common\repository\UserRepositoryInterface;
use app\common\service\FloatSettingsService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class FloatSettings
{
    public function __construct(private UserRepositoryInterface $users, private FloatSettingsService $service)
    {
    }

    public function index()
    {
        return View::fetch('/float', ['user' => $this->users->findById((int) Session::get('user_id'))]);
    }

    public function save(Request $request)
    {
        try {
            $data = $this->service->validate(
                (string) $request->post('float_mode', ''),
                (string) $request->post('float_step', ''),
                (string) $request->post('float_max', ''),
                (int) $request->post('order_timeout', 300),
            );
            $this->users->updateFloat((int) Session::get('user_id'), $data);
            Session::flash('flash', '浮动设置已保存');
        } catch (ValidationException $e) {
            Session::flash('flash', $e->getMessage());
        }
        return redirect('/float');
    }
}
