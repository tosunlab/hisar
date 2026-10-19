package com.tryhisar.hisar.app

/** A refused change: the alert names the action, and its message is the server's sentence. */
data class Refusal(val title: String, val message: String)

/** Takes the sentence of the change that just failed for an alert, instead of leaving it for a screen's error banner. */
fun ModelContext.takeRefusal(title: String): Refusal? {
    val message = errorMessage.value ?: return null
    errorMessage.value = null
    return Refusal(title, message)
}
