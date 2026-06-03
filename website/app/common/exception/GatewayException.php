<?php
namespace app\common\exception;

class GatewayException extends \RuntimeException
{
    public function __construct(public int $errCode, string $message)
    {
        parent::__construct($message);
    }
}
