<?php
// +----------------------------------------------------------------------
// | 控制台配置
// +----------------------------------------------------------------------
return [
    // 指令定义
    'commands' => [
        'vanilla:order-expire' => \app\command\OrderExpire::class,
        'vanilla:device-check' => \app\command\DeviceCheck::class,
        'vanilla:callback-retry' => \app\command\CallbackRetry::class,
        'vanilla:admin-create' => \app\command\AdminCreate::class,
        'vanilla:reconcile-daily' => \app\command\ReconcileDaily::class,
    ],
];
