<?php
namespace app\device\controller;

class Config
{
    public function rules()
    {
        return json([
            'version' => 1,
            'rules' => [
                [
                    'channel' => 'wxpay',
                    'package' => 'com.tencent.mm',
                    'keyword' => '收款',
                    'amountRegex' => '收款([0-9]+(?:\\.[0-9]{1,2})?)元',
                ],
                [
                    'channel' => 'alipay',
                    'package' => 'com.eg.android.AlipayGphone',
                    'keyword' => '收款',
                    'amountRegex' => '收款([0-9]+(?:\\.[0-9]{1,2})?)元',
                ],
            ],
        ]);
    }
}
