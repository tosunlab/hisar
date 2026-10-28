package com.tryhisar.hisar.model

import java.net.URI

/** Setup progress sentences keyed by both the change and its delivery phase. */
object PendingRecoveryCopy {
    private val headings = mapOf(
        "signup" to "Finish creating your account",
        "replace-secret-key" to "Finish replacing your Secret Key",
    )
    private val statuses = mapOf(
        "awaiting-kit" to "Save and confirm your new Emergency Kit before Hisar sends this change.",
        "prepared" to "Your Emergency Kit is confirmed saved. Continue to send this change.",
        "manual-unlock" to "Sign in with your master password and the Secret Key from your saved Emergency Kit.",
        "outcome-unknown" to "Hisar could not confirm whether the server finished this change. Continue to check. Do not start again.",
    )
    private val kindStatuses = mapOf(
        "delivery-pending" to mapOf(
            "signup" to "Your account is ready. Save and confirm your Emergency Kit to finish.",
            "replace-secret-key" to "Your Secret Key was replaced. Save and confirm your new Emergency Kit to finish.",
        ),
        "needs-finishing" to mapOf(
            "signup" to "Your account was created, but setup did not finish on this device. Continue with the same master password.",
            "replace-secret-key" to "Your Secret Key was replaced, but setup did not finish on this device. Continue with the same master password.",
        ),
    )

    fun heading(kind: String): String = headings[kind] ?: "Finish account setup"

    fun status(kind: String, phase: String): String =
        if (kind !in headings) "Finish account setup" else statuses[phase] ?: kindStatuses[phase]?.get(kind) ?: "Finish account setup"

    fun host(origin: String): String = runCatching { URI(origin).authority }.getOrNull() ?: origin
}
