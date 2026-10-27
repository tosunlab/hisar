package com.tryhisar.hisar.ui.approvals

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.unit.Density
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ApprovalDetailTests {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun testLongPlainValuesStackAtLargeFontScale() {
        val rows = listOf(
            PhoneApproval.DetailRow("Warning", "Anyone with this link can read these secrets until it expires.", false),
            PhoneApproval.DetailRow("Secrets", "Personal › GitHub › password\nWork › AWS › access key", false),
            PhoneApproval.DetailRow("Link", "Expires in 7 days · 1 view", false),
        )
        composeRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.5f)) {
                HisarTheme { Column { rows.forEach { DetailRow(it) } } }
            }
        }
        for (row in rows) {
            val label = composeRule.onNodeWithTag("phone-approval-label-${row.label}").fetchSemanticsNode().boundsInRoot
            val value = composeRule.onNodeWithTag("phone-approval-row-${row.label}").fetchSemanticsNode().boundsInRoot
            assertTrue("${row.label} appears above its value", label.bottom <= value.top)
            assertTrue("${row.label} value starts at its label", label.left == value.left)
        }
    }
}
