<?php
namespace app\index\controller;

use app\common\exception\ValidationException;
use app\common\repository\QrcodeRepositoryInterface;
use app\common\service\QrcodeImageProcessor;
use app\common\service\QrcodeUploadValidator;
use think\Request;
use think\facade\Log;
use think\facade\Session;
use think\facade\View;

class Qrcodes
{
    public function __construct(
        private QrcodeRepositoryInterface $qrcodes,
        private QrcodeUploadValidator $validator,
        private QrcodeImageProcessor $processor
    )
    {
    }

    public function index()
    {
        $latest = $this->latestByChannel($this->qrcodes->listByUser((int) Session::get('user_id')));

        return View::fetch('/qrcodes', [
            'wxpayQr' => $latest['wxpay'],
            'alipayQr' => $latest['alipay'],
            'uploadDiagnostics' => $this->uploadDiagnostics(),
        ]);
    }

    public function upload(Request $request)
    {
        $movedPath = '';
        try {
            $userId = (int) Session::get('user_id');
            $channel = (string) $request->post('channel', '');
            $this->validator->validateChannel($channel);
            $file = $request->file('qrcode');
            if (!$file) {
                throw new ValidationException('请选择二维码图片');
            }
            $mime = (string) $file->getMime();
            $this->validator->validate($mime, (int) $file->getSize());
            $saveDir = app()->getRootPath() . 'public/static/uploads/qrcodes';
            if (!is_dir($saveDir)) {
                if (!mkdir($saveDir, 0755, true) && !is_dir($saveDir)) {
                    throw new ValidationException('二维码上传目录创建失败');
                }
            }
            if (!is_writable($saveDir)) {
                throw new ValidationException('二维码上传目录不可写');
            }
            $name = date('YmdHis') . '_' . bin2hex(random_bytes(8)) . '.png';
            $movedPath = $saveDir . DIRECTORY_SEPARATOR . $name;
            $file->move($saveDir, $name);
            $qrContent = $this->processor->process($movedPath, $mime);
            $this->qrcodes->deleteForUserChannel($userId, $channel);
            $this->qrcodes->create([
                'user_id' => $userId,
                'channel' => $channel,
                'name' => $channel,
                'qr_image_path' => '/static/uploads/qrcodes/' . $name,
                'qr_content' => $qrContent,
                'status' => 1,
                'create_time' => date('Y-m-d H:i:s'),
            ]);
            Session::flash('flash', '收款码已上传');
            Session::flash('flash_tone', 'success');
        } catch (\Throwable $e) {
            if ($movedPath !== '' && is_file($movedPath)) {
                @unlink($movedPath);
            }
            Log::error('qrcode upload failed: ' . $e->getMessage());
            Session::flash('flash', '收款码上传失败：' . $e->getMessage());
            Session::flash('flash_tone', 'error');
        }
        return redirect('/qrcodes');
    }

    public function delete(Request $request)
    {
        $this->qrcodes->deleteForUser((int) $request->post('id'), (int) Session::get('user_id'));
        Session::flash('flash', '收款码已删除');
        Session::flash('flash_tone', 'success');
        return redirect('/qrcodes');
    }

    private function uploadDiagnostics(): array
    {
        $uploadRoot = app()->getRootPath() . 'public/static/uploads';
        $saveDir = $uploadRoot . DIRECTORY_SEPARATOR . 'qrcodes';
        $uploadWritable = is_dir($saveDir)
            ? is_writable($saveDir)
            : (is_dir($uploadRoot) && is_writable($uploadRoot));

        return [
            'gd' => extension_loaded('gd'),
            'fileinfo' => extension_loaded('fileinfo'),
            'upload_writable' => $uploadWritable,
            'ready' => extension_loaded('gd') && extension_loaded('fileinfo') && $uploadWritable,
            'upload_max_filesize' => (string) ini_get('upload_max_filesize'),
            'post_max_size' => (string) ini_get('post_max_size'),
        ];
    }

    private function latestByChannel(array $rows): array
    {
        $latest = ['wxpay' => null, 'alipay' => null];
        foreach ($rows as $row) {
            $channel = (string) ($row['channel'] ?? '');
            if (array_key_exists($channel, $latest) && $latest[$channel] === null) {
                $latest[$channel] = $row;
            }
        }

        return $latest;
    }
}
