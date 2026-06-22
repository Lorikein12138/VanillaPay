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
        $this->settings->set('smtp_host', trim((string) $request->post('smtp_host', '')));
        $this->settings->set('smtp_port', trim((string) $request->post('smtp_port', '')));
        $this->settings->set('smtp_secure', trim((string) $request->post('smtp_secure', '')));
        $this->settings->set('smtp_username', trim((string) $request->post('smtp_username', '')));
        $this->settings->set('smtp_from_email', trim((string) $request->post('smtp_from_email', '')));
        $this->settings->set('smtp_from_name', trim((string) $request->post('smtp_from_name', '')));

        $password = (string) $request->post('smtp_password', '');
        if ($password !== '') {
            $this->settings->set('smtp_password', $password);
        }

        $this->settings->set('callback_driver', 'sync');
        Session::flash('flash', '设置已保存');
        return redirect('/console/settings');
    }
}
