package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemDetail

/** What the item editor opens: a new item from a draft (a category, or AutoFill's typed login), or an existing item. */
sealed interface EditorRequest {
    data class New(val vaultID: String, val draft: ItemDetail) : EditorRequest

    data class Edit(val vaultID: String, val itemID: String) : EditorRequest
}
