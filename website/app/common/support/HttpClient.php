<?php
namespace app\common\support;

interface HttpClient
{
    public function postForm(string $url, array $params, int $timeout = 10): HttpResponse;
}
