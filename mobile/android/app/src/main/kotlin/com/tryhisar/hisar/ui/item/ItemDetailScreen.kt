package com.tryhisar.hisar.ui.item

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest

/** One item's detail screen, pushed by the Vaults and Search tabs. */
@Composable
fun ItemDetailScreen(model: AppModel, vaultID: String, itemID: String, onEdit: (EditorRequest) -> Unit, onBack: () -> Unit) {
    Text(itemID)
}
