package com.tryhisar.hisar.ui.item

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.settings.ToggleRow
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

private const val MIN_LENGTH = 8
private const val MAX_LENGTH = 64

/** The generator over the editor, as the iPhone's half-height sheet: Close leaves it, Use fills the password. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PasswordGeneratorSheet(model: AppModel, onUse: (String) -> Unit, onDismiss: () -> Unit) {
    var password by remember { mutableStateOf("") }
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(),
        containerColor = HisarTheme.colors.groupedBackground,
        dragHandle = null,
    ) {
        Row(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ToolbarIconButton(Icons.Filled.Close, "Close", testTag = "generator-close", onClick = onDismiss)
            Text("Password generator", style = HisarType.headline, color = HisarTheme.colors.label, modifier = Modifier.weight(1f), textAlign = TextAlign.Center)
            GlassTextButton("Use", onClick = { onUse(password) }, testTag = "generator-use", enabled = password.isNotEmpty(), isProminent = true)
        }
        Column(
            Modifier
                .weight(1f, fill = false)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp)
                .padding(bottom = 32.dp),
        ) {
            PasswordGeneratorForm(model, password, onPasswordChange = { password = it })
        }
    }
}

/** Settings → Password generator: the same generator as a screen of its own, without Use. */
@Composable
fun StandalonePasswordGeneratorScreen(model: AppModel, onBack: () -> Unit) {
    var password by remember { mutableStateOf("") }
    BackHandler(onBack = onBack)
    InlineTitleForm(
        title = "",
        leading = { ToolbarIconButton(Icons.AutoMirrored.Filled.ArrowBack, "Back", testTag = "generator-back", onClick = onBack) },
        trailing = {},
    ) {
        LargeTitle("Password generator", topPadding = 0.dp)
        PasswordGeneratorForm(model, password, onPasswordChange = { password = it })
    }
}

/** The generated password with Regenerate and Copy, then the options; every change of an option makes a new password. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ColumnScope.PasswordGeneratorForm(model: AppModel, password: String, onPasswordChange: (String) -> Unit) {
    var length by remember { mutableIntStateOf(24) }
    var isDigitIncluded by remember { mutableStateOf(true) }
    var isSymbolIncluded by remember { mutableStateOf(true) }
    var generation by remember { mutableIntStateOf(0) }
    LaunchedEffect(length, isDigitIncluded, isSymbolIncluded, generation) {
        model.itemEditor.generatePassword(length, isDigitIncluded, isSymbolIncluded)?.let(onPasswordChange)
    }
    FormSection {
        FormRow(Modifier.heightIn(min = 44.dp)) {
            SelectionContainer {
                Text(
                    password.ifEmpty { " " },
                    style = HisarType.title3Monospaced,
                    color = HisarTheme.colors.label,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("generated-password"),
                )
            }
        }
        RowDivider()
        EditorButtonRow("Regenerate", Icons.Filled.Refresh, onClick = { generation += 1 }, testTag = "generator-regenerate")
        RowDivider()
        EditorButtonRow("Copy", Icons.Outlined.ContentCopy, onClick = { model.copyText(password, "Password") }, testTag = "generator-copy", enabled = password.isNotEmpty())
    }
    FormSection(header = "Options") {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("Length", style = HisarType.body, color = HisarTheme.colors.label)
            val sliderColors = SliderDefaults.colors(
                activeTrackColor = HisarTheme.colors.tint,
                inactiveTrackColor = HisarTheme.colors.fill,
                activeTickColor = Color.Transparent,
                inactiveTickColor = Color.Transparent,
            )
            Slider(
                value = length.toFloat(),
                onValueChange = { length = it.toInt() },
                valueRange = MIN_LENGTH.toFloat()..MAX_LENGTH.toFloat(),
                steps = MAX_LENGTH - MIN_LENGTH - 1,
                colors = sliderColors,
                thumb = { Box(Modifier.size(28.dp).shadow(3.dp, CircleShape).background(Color.White, CircleShape)) },
                track = { state ->
                    SliderDefaults.Track(
                        sliderState = state,
                        colors = sliderColors,
                        drawStopIndicator = null,
                        drawTick = { _, _ -> },
                        thumbTrackGapSize = 0.dp,
                        trackInsideCornerSize = 0.dp,
                        modifier = Modifier.height(4.dp),
                    )
                },
                modifier = Modifier
                    .weight(1f)
                    .testTag("generator-length")
                    .semantics { contentDescription = "Length" },
            )
            Text("$length", style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.testTag("generator-length-value"))
        }
        RowDivider()
        ToggleRow("Digits", isDigitIncluded, { isDigitIncluded = it }, testTag = "generator-digits")
        RowDivider()
        ToggleRow("Symbols", isSymbolIncluded, { isSymbolIncluded = it }, testTag = "generator-symbols")
    }
}

