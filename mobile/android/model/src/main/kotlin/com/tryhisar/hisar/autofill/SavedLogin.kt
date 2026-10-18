package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.VaultSummary
import kotlinx.coroutines.flow.first

/** A login the user typed into a page or an app, which AutoFill's save prompt offers to keep. */
data class SavedLogin(val username: String, val password: String, val place: Place) {
    sealed interface Place {
        data class Website(val domain: String) : Place

        data class App(val app: AndroidApp, val label: String) : Place
    }

    /** The new login the editor opens with: named after the site or app, with the site as its website. */
    val draft: ItemDetail
        get() = when (place) {
            is Place.Website -> ItemDetail(
                category = ItemDetail.CATEGORY_LOGIN, title = place.domain.removePrefix("www."), username = username, password = password,
                allURL = listOf("https://${place.domain}"),
            )
            is Place.App -> ItemDetail(category = ItemDetail.CATEGORY_LOGIN, title = place.label, username = username, password = password)
        }

    /**
     * Asks the app to open the editor with [draft] in the Personal vault, once Hisar is unlocked and has listed the vaults.
     * A login saved from an app is linked to that app once the user saves it.
     */
    suspend fun openEditor(model: AppModel) {
        val vault = model.unlockedData.first { it.allVault.isNotEmpty() }.allVault.let(::chooseVault) ?: return
        val onSaved: (suspend (ItemDetail) -> Unit)? = (place as? Place.App)?.let { app ->
            { item -> model.perform { it.addAndroidApp(vault.id, item.id, app.app.packageName, app.app.certificateSHA256) } }
        }
        model.requestedEditor.value = EditorRequest.New(vault.id, draft, onSaved)
    }

    private fun chooseVault(allVault: List<VaultSummary>): VaultSummary? =
        allVault.firstOrNull { it.type == VaultSummary.TYPE_PERSONAL } ?: allVault.firstOrNull { it.role != Member.VAULT_ROLE_READER }
}
