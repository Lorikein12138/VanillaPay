package com.vanillapay.monitor.parse

data class NotificationIdentity(
    val postTime: Long,
    val notificationTime: Long,
    val fingerprint: String,
    val groupCount: Int?,
) {
    fun repeats(previous: NotificationIdentity): Boolean {
        if (postTime == previous.postTime) return true
        if (fingerprint != previous.fingerprint) return false
        if (notificationTime > 0 && previous.notificationTime > 0) {
            return notificationTime == previous.notificationTime
        }
        // With no provider timestamp, a count-only update in the same burst
        // represents the last visible receipt rather than a new amount.
        return postTime - previous.postTime in 1..5_000 &&
            groupCount != previous.groupCount && (groupCount != null || previous.groupCount != null)
    }
}
