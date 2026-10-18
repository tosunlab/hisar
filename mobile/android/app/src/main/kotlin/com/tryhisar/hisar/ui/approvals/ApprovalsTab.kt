package com.tryhisar.hisar.ui.approvals

import androidx.compose.runtime.Composable
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.TabScaffold
import com.tryhisar.hisar.ui.root.TabBarHeight

/** The Approvals tab's root; the requests list comes with phone approvals. */
@Composable
fun ApprovalsTab(model: AppModel) {
    TabScaffold(title = "Approvals", bottomInset = TabBarHeight) {}
}
