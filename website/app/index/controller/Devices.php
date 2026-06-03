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
        $list = array_map(function (array $device) use ($serverUrl): array {
            $device['binding_payload'] = rtrim($serverUrl, '/') . '|' . $device['id'] . '|' . $device['device_key'];

            return $device;
        }, $this->devices->listByUser((int) Session::get('user_id')));

        return View::fetch('/devices', ['list' => $list]);
    }

    public function create(Request $request)
    {
        $serverUrl = $request->domain();
        $result = $this->provision->provision((int) Session::get('user_id'), (string) $request->post('name', ''), $serverUrl);
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
