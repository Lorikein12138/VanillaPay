<?php
namespace app\device\controller;

use app\common\repository\DeviceRepositoryInterface;
use app\common\repository\UserRepositoryInterface;
use app\common\service\DeviceSigner;
use app\common\support\Clock;
use think\Request;

class Heart
{
    public function __construct(private DeviceRepositoryInterface $devices, private UserRepositoryInterface $users, private DeviceSigner $signer, private Clock $clock)
    {
    }

    public function beat(Request $request)
    {
        $params = $request->param();
        $device = $this->devices->findById((int) ($params['device_id'] ?? 0));
        if (!$device) {
            return json(['code' => -2, 'msg' => '设备不存在']);
        }
        if (!$this->signer->timestampValid((int) ($params['t'] ?? 0), $this->clock->timestamp())) {
            return json(['code' => -7, 'msg' => '时间戳过期']);
        }
        if (!$this->signer->verify($params, (string) $device['device_key'])) {
            return json(['code' => -1, 'msg' => '签名错误']);
        }

        $this->devices->touchHeartbeat((int) $device['id'], [
            'last_heartbeat' => $this->clock->now(),
            'status' => 'online',
            'last_ip' => $request->ip(),
            'app_version' => $params['app_version'] ?? null,
        ]);

        $merchant = $this->users->findById((int) ($device['user_id'] ?? 0));

        return json([
            'code' => 1,
            'pid' => (string) ($merchant['pid'] ?? ''),
            'server_time' => $this->clock->timestamp(),
            'config' => ['heartbeat_interval' => 30, 'parse_rules_version' => 2],
        ]);
    }
}
