<?php
namespace app\common\service;

use app\common\protocol\AdapterRegistry;
use app\common\repository\CallbackLogRepositoryInterface;
use app\common\repository\OrderRepositoryInterface;
use app\common\repository\UserRepositoryInterface;
use app\common\support\Clock;
use app\common\support\HttpClient;

final class CallbackSender
{
    public function __construct(
        private OrderRepositoryInterface $orders,
        private UserRepositoryInterface $users,
        private CallbackLogRepositoryInterface $logs,
        private AdapterRegistry $adapters,
        private HttpClient $http,
        private Clock $clock,
    ) {
    }

    public function sendForOrder(int $orderId): bool
    {
        $order = $this->orders->findById($orderId);
        if (!$order || empty($order['notify_url'])) {
            return false;
        }
        $user = $this->users->findById((int) $order['user_id']);
        if (!$user) {
            return false;
        }

        $attempts = (int) ($this->logs->findByOrder($orderId)['attempts'] ?? 0) + 1;
        $adapter = $this->adapters->get((string) $order['protocol']);
        $params = $adapter->buildNotifyParams($order, (string) $user['pid'], (string) $user['api_key']);
        $response = $this->http->postForm((string) $order['notify_url'], $params);
        $success = $response->status >= 200
            && $response->status < 300
            && trim(strtolower($response->body)) === strtolower($adapter->successText());

        $nextRetry = null;
        if (!$success) {
            $nextRetry = date('Y-m-d H:i:s', $this->clock->timestamp() + min(3600, 60 * $attempts));
        }

        $this->logs->upsertForOrder($orderId, [
            'url' => (string) $order['notify_url'],
            'request_body' => http_build_query($params),
            'response_body' => $response->body,
            'http_code' => $response->status,
            'success' => $success ? 1 : 0,
            'attempts' => $attempts,
            'next_retry_at' => $nextRetry,
            'update_time' => $this->clock->now(),
        ]);

        $this->orders->update($orderId, [
            'notify_status' => $success ? 1 : 2,
            'notify_count' => $attempts,
        ]);

        return $success;
    }
}
