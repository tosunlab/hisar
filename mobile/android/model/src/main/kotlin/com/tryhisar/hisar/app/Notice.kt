package com.tryhisar.hisar.app

/** A snackbar message and the action available while it is shown. Each instance is a new presentation. */
class Notice(val message: String, val action: Action? = null) {
    class Action(val label: String, val perform: suspend () -> Unit)
}
