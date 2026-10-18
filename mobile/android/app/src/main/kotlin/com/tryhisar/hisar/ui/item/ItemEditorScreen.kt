package com.tryhisar.hisar.ui.item

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest

/** The item editor for a new or an existing item; [onDone] gets the saved item's ID, or null when cancelled. */
@Composable
fun ItemEditorScreen(model: AppModel, request: EditorRequest, onDone: (savedItemID: String?) -> Unit) {
    Text(request.toString())
}
