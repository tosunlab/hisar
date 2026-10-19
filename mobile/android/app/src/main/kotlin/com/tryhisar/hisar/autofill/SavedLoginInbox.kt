package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.app.AppModel
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * Hands the login of a save prompt from the AutoFill service to MainActivity in memory, so the typed password never
 * travels through the system or reaches the disk. MainActivity opens it whenever Hisar comes to the front: the save
 * sheet's [ACTION_SAVE_LOGIN] intent brings it there, and where Android 14 before QPR2 blocks that intent, the user's
 * next visit to Hisar does.
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

    /** Opens the editor for the login the user chose to save, if one waits. */
    fun open(model: AppModel, scope: CoroutineScope) {
        val login = take() ?: return
        scope.launch { login.openEditor(model) }
    }
}
