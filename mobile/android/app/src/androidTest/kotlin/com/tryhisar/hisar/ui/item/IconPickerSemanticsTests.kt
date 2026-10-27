package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.cancel
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

class IconPickerSemanticsTests {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()
    private val scope = MainScope()
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After fun close() { scope.cancel() }

    @Test fun automaticAndPresetsShareOneSelectionGroupAndColorsStayIndependent() {
        var item by mutableStateOf(ItemDetail(title = "Token"))
        rule.setContent { HisarTheme { IconPickerSheet(model, item, { item = item.copy(icon = it) }, {}) } }
        val automatic = rule.onNodeWithTag("icon-automatic")
        automatic.assertIsSelected()
        assertEquals(Role.RadioButton, automatic.fetchSemanticsNode().config[SemanticsProperties.Role])
        rule.onNode(hasTestTag("icon-automatic") and hasAnyAncestor(hasTestTag("icon-source-group"))).assertExists()
        rule.onNode(hasTestTag("icon-preset-code") and hasAnyAncestor(hasTestTag("icon-source-group"))).assertExists()
        rule.onNodeWithTag("icon-preset-code").performScrollTo().performClick().assertIsSelected()
        automatic.assertIsNotSelected()
        rule.onNodeWithTag("icon-color-blue").performScrollTo().performClick().assertIsSelected()
        rule.onNodeWithTag("icon-preset-code").assertIsSelected()
        rule.onNode(hasTestTag("icon-color-blue") and hasAnyAncestor(hasTestTag("icon-color-group"))).assertExists()
        val photo = rule.onNodeWithTag("icon-photo").fetchSemanticsNode()
        assertEquals(Role.Button, photo.config[SemanticsProperties.Role])
        assertNull(photo.config.getOrNull(SemanticsProperties.Selected))
        automatic.performScrollTo().performClick().assertIsSelected()
        rule.onNodeWithTag("icon-preset-code").assertIsNotSelected()
    }
}
