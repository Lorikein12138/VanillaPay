<?php
namespace app\index\controller;

use app\common\support\CurlHttpClient;
use think\Request;
use think\facade\Session;
use think\facade\View;

class CallbackTest
{
    public function index()
    {
        return View::fetch('/callback_test');
    }

    public function send(Request $request)
    {
        $url = (string) $request->post('url', '');
        if (!filter_var($url, FILTER_VALIDATE_URL)) {
            Session::flash('flash', '回调地址不合法');
            return redirect('/callback-test');
        }
        $resp = (new CurlHttpClient())->postForm($url, ['vanillapay_test' => '1', 'time' => time()]);
        $body = function_exists('mb_substr') ? mb_substr($resp->body, 0, 200) : substr($resp->body, 0, 200);
        Session::flash('flash', 'HTTP ' . $resp->status . '：' . $body);
        return redirect('/callback-test');
    }
}
