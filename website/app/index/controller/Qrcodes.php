<?php
namespace app\index\controller;

use app\common\exception\ValidationException;
use app\common\repository\QrcodeRepositoryInterface;
use app\common\service\QrcodeUploadValidator;
use think\Request;
use think\facade\Session;
use think\facade\View;

class Qrcodes
{
    public function __construct(private QrcodeRepositoryInterface $qrcodes, private QrcodeUploadValidator $validator)
    {
    }

    public function index()
    {
        return View::fetch('/qrcodes', ['list' => $this->qrcodes->listByUser((int) Session::get('user_id'))]);
    }

    public function upload(Request $request)
    {
        try {
            $userId = (int) Session::get('user_id');
            $channel = (string) $request->post('channel', '');
            $this->validator->validateChannel($channel);
            $file = $request->file('qrcode');
            if (!$file) {
                throw new ValidationException('请选择二维码图片');
            }
            $this->validator->validate((string) $file->getMime(), (int) $file->getSize());
            $saveDir = app()->getRootPath() . 'public/static/uploads/qrcodes';
            if (!is_dir($saveDir)) {
                mkdir($saveDir, 0755, true);
            }
            $ext = strtolower($file->extension() ?: 'png');
            $name = date('YmdHis') . '_' . bin2hex(random_bytes(8)) . '.' . $ext;
            $file->move($saveDir, $name);
            $this->qrcodes->create([
                'user_id' => $userId,
                'channel' => $channel,
                'name' => (string) $request->post('name', ''),
                'qr_image_path' => '/static/uploads/qrcodes/' . $name,
                'qr_content' => '',
                'status' => 1,
                'create_time' => date('Y-m-d H:i:s'),
            ]);
            Session::flash('flash', '收款码已上传');
        } catch (\Throwable $e) {
            Session::flash('flash', $e->getMessage());
        }
        return redirect('/qrcodes');
    }

    public function delete(Request $request)
    {
        $this->qrcodes->deleteForUser((int) $request->post('id'), (int) Session::get('user_id'));
        Session::flash('flash', '收款码已删除');
        return redirect('/qrcodes');
    }
}
