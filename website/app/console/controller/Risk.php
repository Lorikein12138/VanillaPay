<?php
namespace app\console\controller;

use app\common\repository\RiskEventRepositoryInterface;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Risk
{
    public function __construct(private RiskEventRepositoryInterface $risks)
    {
    }

    public function index(Request $request)
    {
        $page = max(1, (int) $request->get('page', 1));
        return View::fetch('/risk', ['data' => $this->risks->paginate($request->get(), $page, 30), 'query' => $request->get()]);
    }

    public function handle(Request $request)
    {
        $this->risks->markHandled((int) $request->post('id'));
        Session::flash('flash', '风控事件已标记处理');
        return redirect('/console/risk');
    }
}
