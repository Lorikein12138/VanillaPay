<?php
namespace app\common\protocol;

final class YuanpayAdapter extends EpayAdapter
{
    public function code(): string
    {
        return 'yuanpay';
    }
}
