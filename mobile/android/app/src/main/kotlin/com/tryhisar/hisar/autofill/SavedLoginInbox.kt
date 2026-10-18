package com.tryhisar.hisar.autofill

import android.content.Intent
import com.tryhisar.hisar.app.AppModel
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * Hands the login of a save prompt from the AutoFill service to MainActivity in memory. The activity's intent carries only
 * [ACTION_SAVE_LOGIN], so the typed password never travels through the system, and an intent from another app finds
 * nothing to open.
 */
object SavedLoginInbox {
    const val ACTION_SAVE_LOGIN = "com.tryhisar.hisar.action.SAVE_LOGIN"

    private var savedLogin: SavedLogin? = null

    @Synchronized
    fun put(login: SavedLogin) {
        savedLogin = login
    }

    @Synchronized
    private fun take(): SavedLogin? = savedLogin.also { savedLogin = null }

    /** Opens the editor for the saved login when [intent] is the save prompt's. */
    fun open(intent: Intent, model: AppModel, scope: CoroutineScope) {
        if (intent.action != ACTION_SAVE_LOGIN) return
        val login = take() ?: return
        scope.launch { login.openEditor(model) }
    }
}
