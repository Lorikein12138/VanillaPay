<?php
namespace app\common\protocol;

final class AdapterRegistry
{
    /** @var array<string,PayProtocolAdapter> */
    private array $map = [];

    public function __construct(EpayAdapter $epay, CodepayAdapter $codepay, YuanpayAdapter $yuanpay)
    {
        foreach ([$epay, $codepay, $yuanpay] as $adapter) {
            $this->map[$adapter->code()] = $adapter;
        }
    }

    public function get(string $code): PayProtocolAdapter
    {
        return $this->map[$code] ?? throw new \RuntimeException('unknown protocol: ' . $code);
    }
}
