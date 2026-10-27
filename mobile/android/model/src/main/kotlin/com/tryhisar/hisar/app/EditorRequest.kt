package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemDetail

/** What the item editor opens: a new item from a draft (a category, or AutoFill's typed login), or an existing item. */
sealed interface EditorRequest {
    val vaultID: String

    /** [onSaved] runs once the new item is saved, with the saved item; AutoFill links the app the login was typed into. */
    data class New(override val vaultID: String, val draft: ItemDetail, val onSaved: (suspend (String, ItemDetail) -> Unit)? = null) : EditorRequest

    data class Edit(override val vaultID: String, val itemID: String) : EditorRequest
}
