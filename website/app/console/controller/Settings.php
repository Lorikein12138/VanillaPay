<?php
namespace app\console\controller;

use app\common\repository\SettingsRepositoryInterface;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Settings
{
    public function __construct(private SettingsRepositoryInterface $settings)
    {
    }

    public function index()
    {
        return View::fetch('/settings', ['s' => $this->settings->all()]);
    }

    public function save(Request $request)
    {
        $this->settings->set('register_audit', (string) (int) $request->post('register_audit', 0));
        $this->settings->set('callback_driver', (string) $request->post('callback_driver', 'sync'));
        $this->settings->set('notice', (string) $request->post('notice', ''));
        Session::flash('flash', '设置已保存');
        return redirect('/console/settings');
    }
}
