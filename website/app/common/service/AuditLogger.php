<?php
namespace app\common\service;

use app\common\repository\LoginLogRepositoryInterface;
use app\common\repository\OperationLogRepositoryInterface;

final class AuditLogger
{
    public function __construct(private LoginLogRepositoryInterface $loginLogs, private OperationLogRepositoryInterface $operationLogs)
    {
    }

    public function login(string $userType, int $uid, string $ip, string $ua, string $result): void
    {
        $this->loginLogs->record([
            'user_type' => $userType,
            'uid' => $uid,
            'ip' => $ip,
            'ua' => function_exists('mb_substr') ? mb_substr($ua, 0, 255) : substr($ua, 0, 255),
            'result' => $result,
        ]);
    }

    public function operation(string $actorType, int $actorId, string $action, string $target, string $ip, string $detail = ''): void
    {
        $this->operationLogs->record([
            'actor_type' => $actorType,
            'actor_id' => $actorId,
            'action' => $action,
            'target' => function_exists('mb_substr') ? mb_substr($target, 0, 120) : substr($target, 0, 120),
            'ip' => $ip,
            'detail' => $detail,
        ]);
    }
}
