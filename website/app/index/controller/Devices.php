<?php
namespace app\index\controller;

use app\common\repository\DeviceRepositoryInterface;
use app\common\service\DeviceProvisionService;
use chillerlan\QRCode\QRCode;
use chillerlan\QRCode\QROptions;
use chillerlan\QRCode\Output\QRGdImagePNG;
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
        $currentDevice = $this->ensureDevice((int) Session::get('user_id'), $serverUrl);

        return View::fetch('/devices', ['currentDevice' => $currentDevice]);
    }

    public function create(Request $request)
    {
        $userId = (int) Session::get('user_id');
        $currentDevice = $this->devices->listByUser($userId)[0] ?? null;
        if ($currentDevice) {
            Session::flash('flash', '当前商户已有监控设备。');
            return redirect('/devices');
        }

        $serverUrl = $request->domain();
        $result = $this->provision->provision($userId, '', $serverUrl);
        Session::flash('flash', '设备已创建，绑定串：' . $result['binding_payload']);
        return redirect('/devices');
    }

    public function delete(Request $request)
    {
        $this->devices->deleteForUser((int) $request->post('id'), (int) Session::get('user_id'));
        Session::flash('flash', '设备已删除');
        return redirect('/devices');
    }

    private function bindingQrDataUri(string $payload): string
    {
        try {
            return (string) (new QRCode(new QROptions([
                'outputInterface' => QRGdImagePNG::class,
                'outputBase64' => true,
                'scale' => 8,
                'quietzoneSize' => 2,
                'eccLevel' => 'M',
            ])))->render($payload);
        } catch (\Throwable) {
            return '';
        }
    }

    private function ensureDevice(int $userId, string $serverUrl): array
    {
        $currentDevice = $this->devices->listByUser($userId)[0] ?? null;
        if (!$currentDevice) {
            $result = $this->provision->provision($userId, '', $serverUrl);
            $currentDevice = $this->devices->findById((int) $result['device_id']) ?? [
                'id' => $result['device_id'],
                'device_key' => $result['device_key'],
                'status' => 'offline',
                'last_heartbeat' => null,
                'app_version' => null,
            ];
        }

        $currentDevice['binding_payload'] = rtrim($serverUrl, '/') . '|' . $currentDevice['id'] . '|' . $currentDevice['device_key'];
        $currentDevice['binding_qr'] = $this->bindingQrDataUri($currentDevice['binding_payload']);
        $currentDevice['is_bound'] = !empty($currentDevice['last_heartbeat']);

        return $currentDevice;
    }
}
