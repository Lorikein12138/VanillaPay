<?php
namespace app\console\controller;

use app\common\repository\SettingsRepositoryInterface;
use think\Request;
use think\facade\Db;
use think\facade\Session;
use think\facade\View;

class Channels
{
    public function __construct(private SettingsRepositoryInterface $settings)
    {
    }

    public function index()
    {
        return View::fetch('/channels', [
            'channels' => Db::name('channels')->select()->toArray(),
            'protocols' => [
                'epay' => $this->settings->get('proto_epay', '1'),
                'codepay' => $this->settings->get('proto_codepay', '1'),
                'yuanpay' => $this->settings->get('proto_yuanpay', '1'),
            ],
        ]);
    }

    public function save(Request $request)
    {
        foreach (['wxpay', 'alipay'] as $channel) {
            Db::name('channels')->where('code', $channel)->update(['enabled' => (int) $request->post($channel, 0)]);
        }
        foreach (['epay', 'codepay', 'yuanpay'] as $protocol) {
            $this->settings->set('proto_' . $protocol, (string) (int) $request->post($protocol, 0));
        }
        Session::flash('flash', '渠道与协议已保存');
        return redirect('/console/channels');
    }
}
