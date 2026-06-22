<?php
namespace app\common\service;

final class EmailTemplateRenderer
{
    /**
     * @return array{subject:string,html:string,text:string}
     */
    public static function verificationCode(string $scene, string $code, int $ttlMinutes): array
    {
        $safeScene = htmlspecialchars($scene, ENT_QUOTES, 'UTF-8');
        $safeCode = htmlspecialchars($code, ENT_QUOTES, 'UTF-8');
        $subject = 'VanillaPay ' . $scene . '验证码';
        $html = <<<HTML
<!doctype html>
<html lang="zh-CN">
<body style="margin:0;background:#f3f7f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Arial,'PingFang SC','Microsoft YaHei',sans-serif;color:#18181b;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f3f7f6;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:520px;background:#ffffff;border:1px solid #e4e4e7;border-radius:18px;box-shadow:0 22px 60px rgba(15,23,42,.10);overflow:hidden;">
          <tr>
            <td style="padding:28px 28px 12px;">
              <div style="font-size:14px;font-weight:700;color:#0f766e;letter-spacing:.04em;">VanillaPay</div>
              <h1 style="margin:14px 0 0;font-size:26px;line-height:1.25;color:#09090b;">{$safeScene}验证码</h1>
              <p style="margin:12px 0 0;font-size:14px;line-height:1.8;color:#71717a;">请在页面中输入以下验证码完成操作。验证码仅用于本次验证，请勿转发给他人。</p>
            </td>
          </tr>
          <tr>
            <td style="padding:12px 28px 8px;">
              <div style="border-radius:14px;background:#ecfdf5;border:1px solid #99f6e4;padding:22px;text-align:center;">
                <div style="font-size:34px;line-height:1;font-weight:800;letter-spacing:8px;color:#0f172a;">{$safeCode}</div>
              </div>
            </td>
          </tr>
          <tr>
            <td style="padding:12px 28px 28px;">
              <p style="margin:0;font-size:13px;line-height:1.7;color:#71717a;">验证码 {$ttlMinutes} 分钟内有效。如果不是你本人操作，可以忽略这封邮件。</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>
HTML;
        $text = "VanillaPay {$scene}验证码：{$code}，{$ttlMinutes} 分钟内有效。";

        return ['subject' => $subject, 'html' => $html, 'text' => $text];
    }
}
