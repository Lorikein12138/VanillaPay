<?php
use app\ExceptionHandle;
use app\Request;

// 容器Provider定义文件
return [
    'think\Request'          => Request::class,
    'think\exception\Handle' => ExceptionHandle::class,
    \app\common\repository\UserRepositoryInterface::class => \app\common\repository\ThinkUserRepository::class,
    \app\common\repository\OrderRepositoryInterface::class => \app\common\repository\ThinkOrderRepository::class,
    \app\common\repository\AmountLockRepositoryInterface::class => \app\common\repository\ThinkAmountLockRepository::class,
    \app\common\repository\QrcodeRepositoryInterface::class => \app\common\repository\ThinkQrcodeRepository::class,
    \app\common\repository\DeviceRepositoryInterface::class => \app\common\repository\ThinkDeviceRepository::class,
    \app\common\repository\RiskEventRepositoryInterface::class => \app\common\repository\ThinkRiskEventRepository::class,
    \app\common\repository\CallbackLogRepositoryInterface::class => \app\common\repository\ThinkCallbackLogRepository::class,
    \app\common\repository\AdminRepositoryInterface::class => \app\common\repository\ThinkAdminRepository::class,
    \app\common\repository\SettingsRepositoryInterface::class => \app\common\repository\ThinkSettingsRepository::class,
    \app\common\repository\LoginLogRepositoryInterface::class => \app\common\repository\ThinkLoginLogRepository::class,
    \app\common\repository\OperationLogRepositoryInterface::class => \app\common\repository\ThinkOperationLogRepository::class,
    \app\common\support\Clock::class => \app\common\support\SystemClock::class,
    \app\common\support\HttpClient::class => \app\common\support\CurlHttpClient::class,
    \app\common\support\CacheStore::class => \app\common\support\ThinkCacheStore::class,
    \app\common\service\MailerInterface::class => \app\common\service\SmtpMailer::class,
    \app\common\contract\OrderPaidHandler::class => \app\common\contract\CallbackDispatcher::class,
];
