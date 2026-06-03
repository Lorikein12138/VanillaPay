<?php
namespace app\index\controller;

use app\common\repository\DeviceRepositoryInterface;
use app\common\service\DeviceProvisionService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Devices
{
    public function __construct(private DeviceRepositoryInterface $devices, private DeviceProvisionService $provision)
    {
    }

    public function index(Request $request)
    {
        $serverUrl = $request->domain();
        $currentDevice = $this->devices->listByUser((int) Session::get('user_id'))[0] ?? null;
        if ($currentDevice) {
            $currentDevice['binding_payload'] = rtrim($serverUrl, '/') . '|' . $currentDevice['id'] . '|' . $currentDevice['device_key'];
        }

        return View::fetch('/devices', ['currentDevice' => $currentDevice]);
    }

    public function create(Request $request)
    {
        $userId = (int) Session::get('user_id');
        if ($this->devices->listByUser($userId) !== []) {
            Session::flash('flash', '已有设备。每个商户只能绑定一个设备，重新绑定前请先删除当前设备。');
            return redirect('/devices');
        }

        $serverUrl = $request->domain();
        $result = $this->provision->provision($userId, (string) $request->post('name', ''), $serverUrl);
        Session::flash('flash', '设备已创建，绑定串：' . $result['binding_payload']);
        return redirect('/devices');
    }

    public function delete(Request $request)
    {
        $this->devices->deleteForUser((int) $request->post('id'), (int) Session::get('user_id'));
        Session::flash('flash', '设备已删除');
        return redirect('/devices');
    }
}
