<?php
namespace app\console\controller;

use app\common\service\ReconciliationService;
use think\Request;
use think\facade\View;

class Reconcile
{
    public function __construct(private ReconciliationService $service)
    {
    }

    public function index(Request $request)
    {
        $date = (string) $request->get('date', date('Y-m-d'));
        return View::fetch('/reconcile', ['date' => $date, 'r' => $this->service->daily($date)]);
    }
}
