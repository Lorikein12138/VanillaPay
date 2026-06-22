<?php
namespace app\common\service;

use app\common\repository\OrderRepositoryInterface;
use app\common\repository\RiskEventRepositoryInterface;

final class ReconciliationService
{
    public function __construct(private OrderRepositoryInterface $orders, private RiskEventRepositoryInterface $risks)
    {
    }

    public function daily(string $date): array
    {
        $start = $date . ' 00:00:00';
        $end = $date . ' 23:59:59';
        return [
            'date' => $date,
            'paid_count' => $this->orders->countByStatusBetween('paid', $start, $end),
            'paid_amount' => $this->orders->sumPaidBetween($start, $end),
            'expired_count' => $this->orders->countByStatusBetween('expired', $start, $end),
            'notify_fail' => $this->orders->countNotifyFailBetween($start, $end),
            'unmatched_count' => $this->risks->countByTypeBetween('unmatched_payment', $start, $end),
        ];
    }
}
