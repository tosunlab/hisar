package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemSoftwareLicense
import kotlinx.io.bytestring.ByteString
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

data class ItemEditorSession(
    val request: EditorRequest,
    val draft: ItemEditorDraft,
    val chosenVaultID: String,
    val sourceVaultID: String = request.vaultID,
    val needsRestoration: Boolean = false,
    val recoveryMessage: String? = null,
)

/** Creating, editing and deleting items, the password generator and icon photos: the editor's half of the app model. */
class ItemEditorModel(private val context: ModelContext) {
    private val mutableSession = MutableStateFlow<ItemEditorSession?>(null)
    val session = mutableSession.asStateFlow()

    init {
        context.addStatusListener { previous, current ->
            if (!current.isSignedIn || previous.email != current.email || previous.server != current.server) {
                discard()
            } else if (previous.isUnlocked && !current.isUnlocked) {
                mutableSession.value = mutableSession.value?.copy(needsRestoration = true, recoveryMessage = null)
            }
        }
    }

    suspend fun open(request: EditorRequest) {
        if (!context.status.value.isUnlocked || mutableSession.value?.request === request) return
        val account = context.status.value
        val item = when (request) {
            is EditorRequest.New -> request.draft
            is EditorRequest.Edit -> loadItem(request.vaultID, request.itemID) ?: return
        }
        if (!context.status.value.isUnlocked || context.status.value.email != account.email || context.status.value.server != account.server) return
        mutableSession.value = ItemEditorSession(request, ItemEditorDraft.open(item, request is EditorRequest.New), request.vaultID)
    }

    fun updateDraft(draft: ItemEditorDraft) {
        if (context.status.value.isUnlocked) mutableSession.value = mutableSession.value?.copy(draft = draft)
    }

    fun chooseVault(vaultID: String) {
        mutableSession.value = mutableSession.value?.copy(chosenVaultID = vaultID)
    }

    fun discard() {
        mutableSession.value = null
    }

    fun validateRestoration() {
        val saved = mutableSession.value ?: return
        if (!saved.needsRestoration || !context.status.value.isUnlocked) return
        val data = context.unlockedData.value
        val source = data.findVault(saved.sourceVaultID)
        val destination = data.findVault(saved.chosenVaultID)
        val original = saved.draft.original
        val current = data.allItemByVaultID[saved.sourceVaultID]?.firstOrNull { it.id == original?.id }
        val isChanged = source == null || !CredentialImportModel.isWritableVault(source) || destination == null || !CredentialImportModel.isWritableVault(destination) ||
            (original != null && (current?.revision != original.revision || current.readIssue != null))
        mutableSession.value = saved.copy(
            needsRestoration = false,
            recoveryMessage = if (isChanged) "The original item or your access changed. Your unsaved changes are still here." else null,
        )
    }

    fun recoverCopy() {
        val saved = mutableSession.value ?: return
        val destination = context.unlockedData.value.allVault.firstOrNull(CredentialImportModel::isWritableVault) ?: return
        val item = saved.draft.item.copy(id = "", revision = "", allPasskey = emptyList())
        mutableSession.value = saved.copy(
            draft = saved.draft.copy(original = null, item = item),
            chosenVaultID = destination.id,
            sourceVaultID = destination.id,
            recoveryMessage = null,
        )
    }

    /** The item an Edit request opens, with its secrets; null with the reason in the error banner. */
    suspend fun loadItem(vaultID: String, itemID: String): ItemDetail? =
        context.perform(isBusyShown = false) { it.getItem(vaultID, itemID) }

    /** Saves the item and lists it without its secrets, as the backend's list does. The editor closes once it is saved. */
    suspend fun saveItem(vaultID: String, item: ItemDetail): ItemDetail? = context.runInModel {
        val savedItem = context.perform { it.saveItem(vaultID, item) } ?: return@runInModel null
        val summary = summarize(savedItem)
        context.updateUnlockedData { data ->
            val allItem = data.allItemByVaultID[vaultID].orEmpty()
            val index = allItem.indexOfFirst { it.id == savedItem.id }
            val newAllItem = if (index >= 0) allItem.toMutableList().apply { set(index, summary) } else allItem + summary
            data.copy(allItemByVaultID = data.allItemByVaultID + (vaultID to newAllItem))
        }
        discard()
        savedItem
    }

    /** Deletes the item; the editor and the item's detail close once it is gone. */
    suspend fun deleteItem(vaultID: String, itemID: String): Boolean = context.runInModel {
        context.perform { it.deleteItem(vaultID, itemID) } ?: return@runInModel false
        context.updateUnlockedData { data ->
            val allItem = data.allItemByVaultID[vaultID]
            data.copy(
                allItemByVaultID = if (allItem == null) data.allItemByVaultID else data.allItemByVaultID + (vaultID to allItem.filter { it.id != itemID }),
                allPasskey = data.allPasskey.filterNot { it.vaultID == vaultID && it.itemID == itemID },
            )
        }
        discard()
        true
    }

    /** The brand, last four digits and warnings for a card being typed; null when the backend cannot answer. */
    suspend fun describeCard(number: String, expiry: String): CardDescription? = context.attempt { it.describeCard(number, expiry) }

    suspend fun generatePassword(length: Int, isDigitIncluded: Boolean, isSymbolIncluded: Boolean): String? =
        context.perform(isBusyShown = false) { it.generatePassword(length, isDigitIncluded, isSymbolIncluded) }

    /**
     * Turns a chosen photo into the icon image an item keeps, or shows why it can't. Photos come as the library keeps them
     * (HEIF from the camera, BMP, PNG…), which the backend does not all read, so [convertPhoto] decodes and shrinks the
     * photo first into a PNG or JPEG, or gives null for data that is no image it can read.
     */
    suspend fun prepareItemIcon(photo: ByteString, convertPhoto: (ByteString) -> ByteString?): ByteString? =
        context.perform { backend ->
            val upload = convertPhoto(photo) ?: throw BackendError(UNREADABLE_PHOTO_MESSAGE)
            backend.prepareItemIcon(upload)
        }

    fun clearError() {
        context.errorMessage.value = null
    }

    companion object {
        const val UNREADABLE_PHOTO_MESSAGE = "Hisar can't read this photo. Choose another one."

        /** The item as the backend lists it: what rows and search show, never a secret. */
        private fun summarize(item: ItemDetail): ItemDetail = item.copy(
            password = "",
            notes = "",
            allField = emptyList(),
            oneTimePassword = "",
            card = item.card?.let { ItemCard(type = it.type, brand = it.brand, last4 = it.last4) },
            identity = item.identity?.let {
                ItemIdentity(
                    firstName = it.firstName, middleName = it.middleName, lastName = it.lastName, email = it.email,
                    street = it.street.firstLine(), city = it.city, country = it.country,
                )
            },
            apiCredential = item.apiCredential?.let { ItemAPICredential(service = it.service, keyID = it.keyID, host = it.host) },
            address = item.address?.let { ItemAddress(street = it.street.firstLine(), city = it.city, country = it.country) },
            server = item.server?.let { ItemServer(host = it.host, port = it.port) },
            database = item.database?.let { ItemDatabase(engine = it.engine, engineName = it.engineName, host = it.host, port = it.port, name = it.name) },
            softwareLicense = item.softwareLicense?.let {
                ItemSoftwareLicense(product = it.product, version = it.version, licensedTo = it.licensedTo, email = it.email)
            },
        )

        private fun String.firstLine(): String = split("\n", limit = 2).first()
    }
}
