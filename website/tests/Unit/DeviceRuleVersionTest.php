<?php
use PHPUnit\Framework\TestCase;

final class DeviceRuleVersionTest extends TestCase
{
    public function testHeartbeatAdvertisesCurrentConfigRuleVersion(): void
    {
        $root = dirname(__DIR__, 2);
        $heart = file_get_contents($root . '/app/device/controller/Heart.php') ?: '';
        $config = file_get_contents($root . '/app/device/controller/Config.php') ?: '';

        preg_match("/'parse_rules_version'\\s*=>\\s*(\\d+)/", $heart, $heartMatches);
        preg_match("/'version'\\s*=>\\s*(\\d+)/", $config, $configMatches);

        $this->assertNotEmpty($heartMatches, 'Heartbeat response must advertise parse_rules_version.');
        $this->assertNotEmpty($configMatches, 'Config response must expose the rule version.');
        $this->assertSame($configMatches[1], $heartMatches[1]);
    }
}
