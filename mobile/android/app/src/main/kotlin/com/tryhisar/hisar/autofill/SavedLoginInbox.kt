package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.app.AppModel
import kotlinx.coroutines.launch

/**
 * Hands the logins of save prompts from the AutoFill service to MainActivity in memory, so the typed passwords never
 * travel through the system or reach the disk. MainActivity opens them whenever Hisar comes to the front: the save
 * sheet's [ACTION_SAVE_LOGIN] intent brings it there, and where Android 14 before QPR2 blocks that intent, the user's
 * next visit to Hisar does. Every accepted save waits, in the order the user accepted them.
 */
object SavedLoginInbox {
    const val ACTION_SAVE_LOGIN = "com.tryhisar.hisar.action.SAVE_LOGIN"

    private val allSavedLogin = mutableListOf<SavedLogin>()

    @Synchronized
    fun put(login: SavedLogin) {
        allSavedLogin += login
    }

    @Synchronized
    private fun takeAll(): List<SavedLogin> = allSavedLogin.toList().also { allSavedLogin.clear() }

    /** Asks the app model for an editor for each login the user chose to save; they open one after another. */
    fun open(model: AppModel) {
        val allLogin = takeAll().ifEmpty { return }
        model.scope.launch { allLogin.forEach { it.openEditor(model) } }
    }
}
