<?php
namespace app\common\dto;

final class MatchResult
{
    public const MATCHED = 'matched';
    public const UNMATCHED = 'unmatched';
    public const ALREADY_DONE = 'already_done';

    public function __construct(public string $status, public ?array $order = null)
    {
    }

    public static function matched(?array $order): self
    {
        return new self(self::MATCHED, $order);
    }

    public static function unmatched(): self
    {
        return new self(self::UNMATCHED);
    }

    public static function alreadyDone(array $order): self
    {
        return new self(self::ALREADY_DONE, $order);
    }

    public function isMatched(): bool
    {
        return $this->status === self::MATCHED;
    }

    public function isSettled(): bool
    {
        return in_array($this->status, [self::MATCHED, self::ALREADY_DONE], true);
    }

    public function isUnmatched(): bool
    {
        return $this->status === self::UNMATCHED;
    }

    public function isAlreadyDone(): bool
    {
        return $this->status === self::ALREADY_DONE;
    }
}
