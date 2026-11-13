package platform

import "slices"

func PaymentChannels() []string { return []string{"alipay", "wxpay"} }

func ValidPaymentChannel(channel string) bool { return slices.Contains(PaymentChannels(), channel) }
