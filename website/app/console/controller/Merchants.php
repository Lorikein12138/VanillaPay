<?php
namespace app\console\controller;

use app\common\exception\ValidationException;
use app\common\repository\UserRepositoryInterface;
use app\common\service\AuditLogger;
use app\common\service\CredentialGenerator;
use app\common\service\FloatSettingsService;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Merchants
{
    public function __construct(private UserRepositoryInterface $users, private FloatSettingsService $float, private AuditLogger $audit)
    {
    }

    public function index(Request $request)
    {
        $page = max(1, (int) $request->get('page', 1));
        return View::fetch('/merchants', [
            'data' => $this->users->paginate($request->get(), $page, 20),
            'query' => $request->get(),
        ]);
    }

    public function setStatus(Request $request)
    {
        $id = (int) $request->post('id');
        $status = (int) $request->post('status');
        $this->users->setStatus($id, $status);
        $this->audit->operation('admin', (int) Session::get('admin_id'), 'merchant.status', 'uid=' . $id, $request->ip(), 'status=' . $status);
        Session::flash('flash', '商户状态已更新');
        return redirect('/console/merchants');
    }

    public function resetKey(Request $request)
    {
        $id = (int) $request->post('id');
        $this->users->resetApiKey($id, (new CredentialGenerator())->apiKey());
        $this->audit->operation('admin', (int) Session::get('admin_id'), 'merchant.reset_key', 'uid=' . $id, $request->ip());
        Session::flash('flash', 'API KEY 已重置');
        return redirect('/console/merchants');
    }

    public function saveFloat(Request $request)
    {
        try {
            $data = $this->float->validate(
                (string) $request->post('float_mode', ''),
                (string) $request->post('float_step', ''),
                (string) $request->post('float_max', ''),
                (int) $request->post('order_timeout', 300),
            );
            $this->users->updateFloat((int) $request->post('id'), $data);
            Session::flash('flash', '浮动设置已更新');
        } catch (ValidationException $e) {
            Session::flash('flash', $e->getMessage());
        }
        return redirect('/console/merchants');
    }
}
