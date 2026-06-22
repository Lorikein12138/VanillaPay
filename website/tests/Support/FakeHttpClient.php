<?php
namespace tests\Support;

use app\common\support\HttpClient;
use app\common\support\HttpResponse;

final class FakeHttpClient implements HttpClient
{
    public HttpResponse $response;
    public array $requests = [];

    public function __construct(?HttpResponse $response = null)
    {
        $this->response = $response ?? new HttpResponse(200, 'success');
    }

    public function postForm(string $url, array $params, int $timeout = 10): HttpResponse
    {
        $this->requests[] = ['url' => $url, 'params' => $params, 'timeout' => $timeout];
        return $this->response;
    }
}
