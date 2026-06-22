<?php
namespace app\common\support;

final class HttpResponse
{
    public function __construct(public int $status, public string $body)
    {
    }
}
