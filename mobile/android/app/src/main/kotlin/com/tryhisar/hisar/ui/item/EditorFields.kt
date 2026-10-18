package com.tryhisar.hisar.ui.item

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.RemoveCircleOutline
import androidx.compose.material.icons.outlined.UnfoldMore
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDefaults
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.SelectableDates
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.CalendarDate
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.components.SecretInput
import com.tryhisar.hisar.ui.components.secretInput
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

private val RowInset = 16.dp
private val RowMinHeight = 52.dp

/**
 * Typing options of a field: what keyboard, whether it corrects and capitalizes, as SwiftUI's modifiers set them. A
 * secret kind also keeps its text from the keyboard's learning and from autofill.
 */
enum class FieldKind(val keyboardOptions: KeyboardOptions, val isSecret: Boolean = false) {
    TEXT(KeyboardOptions(capitalization = KeyboardCapitalization.Sentences)),
    NAME(KeyboardOptions(capitalization = KeyboardCapitalization.Words)),
    PLAIN(KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false)),
    URL(KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false, keyboardType = KeyboardType.Uri)),
    EMAIL(KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false, keyboardType = KeyboardType.Email)),
    PHONE(KeyboardOptions(keyboardType = KeyboardType.Phone)),
    NUMBER(KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Number)),
    SECRET(KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false, keyboardType = KeyboardType.Password), isSecret = true),
    SECRET_NUMBER(KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.NumberPassword), isSecret = true),
    SECRET_UPPERCASE(KeyboardOptions(capitalization = KeyboardCapitalization.Characters, autoCorrectEnabled = false, keyboardType = KeyboardType.Password), isSecret = true),
    POSTAL_CODE(KeyboardOptions(capitalization = KeyboardCapitalization.Characters)),
}

/**
 * The editor's screen: an inline title between Cancel and Save, as the iPhone editor's sheet, over grouped sections that
 * scroll above the keyboard.
 */
@Composable
fun InlineTitleForm(
    title: String,
    leading: @Composable RowScope.() -> Unit,
    trailing: @Composable RowScope.() -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .imePadding(),
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .windowInsetsPadding(WindowInsets.statusBars)
                .height(60.dp)
                .padding(horizontal = 16.dp),
        ) {
            Row(Modifier.align(Alignment.CenterStart), verticalAlignment = Alignment.CenterVertically, content = leading)
            Text(
                title,
                style = HisarType.headline,
                color = HisarTheme.colors.label,
                textAlign = TextAlign.Center,
                maxLines = 1,
                modifier = Modifier
                    .align(Alignment.Center)
                    .padding(horizontal = 96.dp),
            )
            Row(Modifier.align(Alignment.CenterEnd), verticalAlignment = Alignment.CenterVertically, content = trailing)
        }
        Column(
            Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin),
        ) {
            content()
            Spacer(Modifier.height(24.dp))
            Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
        }
    }
}

/** A toolbar text button; [isProminent] is the confirming action, as Save. */
@Composable
fun ToolbarTextButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true, isProminent: Boolean = false, color: Color = HisarTheme.colors.tint) {
    Box(
        Modifier
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Text(
            text,
            style = HisarType.body,
            fontWeight = if (isProminent) FontWeight.SemiBold else FontWeight.Normal,
            color = if (enabled) color else HisarTheme.colors.secondaryLabel,
        )
    }
}

/** A toolbar text button in a glass capsule, as the iPhone app's Cancel, Save and Done. */
@Composable
fun GlassTextButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean = true, isProminent: Boolean = false) {
    Box(
        Modifier
            .height(44.dp)
            .shadow(6.dp, CircleShape)
            .clip(CircleShape)
            .background(HisarTheme.colors.cellBackground)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 18.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text,
            style = HisarType.body,
            fontWeight = if (isProminent) FontWeight.SemiBold else FontWeight.Normal,
            color = if (enabled) HisarTheme.colors.label else HisarTheme.colors.secondaryLabel,
        )
    }
}

@Composable
fun ToolbarSpinner() {
    Box(Modifier.padding(horizontal = 16.dp, vertical = 10.dp)) {
        CircularProgressIndicator(Modifier.size(22.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
    }
}

/**
 * A text field on one row, or on several lines up to [maxLines], with a grey placeholder; a secret one hides its text
 * unless [isRevealed], and [trailing] holds its row's buttons.
 */
@Composable
fun EditorField(
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    testTag: String,
    modifier: Modifier = Modifier,
    kind: FieldKind = FieldKind.TEXT,
    isSecret: Boolean = false,
    isRevealed: Boolean = false,
    isMonospaced: Boolean = false,
    minLines: Int = 1,
    maxLines: Int = 1,
    textStyle: TextStyle = HisarType.body,
    imeAction: ImeAction = ImeAction.Default,
    onImeAction: (() -> Unit)? = null,
    trailing: (@Composable RowScope.() -> Unit)? = null,
) {
    val style = if (isMonospaced) textStyle.copy(fontFamily = HisarType.bodyMonospaced.fontFamily) else textStyle
    val isHidden = isSecret && !isRevealed
    val isSecretInput = isSecret || kind.isSecret
    Row(
        modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .padding(start = RowInset, end = if (trailing == null) RowInset else 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SecretInput(isSecretInput) {
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = maxLines == 1,
                minLines = minLines,
                maxLines = maxLines,
                textStyle = style.copy(color = HisarTheme.colors.label),
                cursorBrush = SolidColor(HisarTheme.colors.tint),
                visualTransformation = if (isHidden) PasswordVisualTransformation() else VisualTransformation.None,
                keyboardOptions = kind.keyboardOptions.copy(imeAction = imeAction),
                keyboardActions = if (onImeAction == null) KeyboardActions.Default else KeyboardActions(onAny = { onImeAction() }),
                modifier = Modifier
                    .weight(1f)
                    .padding(vertical = 14.dp)
                    .secretInput(isSecretInput)
                    .testTag(testTag),
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty()) Text(placeholder, style = style, color = HisarTheme.colors.placeholder)
                        innerTextField()
                    }
                },
            )
        }
        if (trailing != null) trailing()
    }
}

/** A borderless symbol button on a field's row, as the reveal and generator buttons of the iPhone editor. */
@Composable
fun RowIconButton(icon: ImageVector, contentDescription: String, onClick: () -> Unit, testTag: String = "", tint: Color = HisarTheme.colors.secondaryLabel) {
    Box(
        Modifier
            .size(44.dp)
            .clickable(role = Role.Button, onClick = onClick)
            .semantics { this.contentDescription = contentDescription }
            .then(if (testTag.isEmpty()) Modifier else Modifier.testTag(testTag)),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
    }
}

/** The eye that reveals or hides one secret field. */
@Composable
fun RevealButton(isRevealed: Boolean, name: String, onToggle: () -> Unit) {
    RowIconButton(
        if (isRevealed) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
        contentDescription = if (isRevealed) "Hide $name" else "Reveal $name",
        onClick = onToggle,
    )
}

/** A Form row whose label leads and whose control trails, as a SwiftUI Toggle. */
@Composable
fun ToggleRow(label: String, isOn: Boolean, onChange: (Boolean) -> Unit, testTag: String, textStyle: TextStyle = HisarType.body, color: Color = HisarTheme.colors.label) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 44.dp)
            .clickable(role = Role.Switch) { onChange(!isOn) }
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = textStyle, color = color, modifier = Modifier.weight(1f))
        Switch(
            checked = isOn,
            onCheckedChange = null,
            colors = SwitchDefaults.colors(
                checkedTrackColor = Color(0xFF34C759),
                checkedThumbColor = Color.White,
                checkedBorderColor = Color.Transparent,
                uncheckedTrackColor = HisarTheme.colors.fill,
                uncheckedThumbColor = Color.White,
                uncheckedBorderColor = Color.Transparent,
            ),
        )
    }
}

/** A choice from a menu on one row, as a Form Picker: the label leads, the chosen option trails with up-down chevrons. */
@Composable
fun <Option> MenuPickerRow(label: String, allOption: List<Option>, selected: Option, describe: (Option) -> String, onSelect: (Option) -> Unit, testTag: String) {
    var isExpanded by remember { mutableStateOf(false) }
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = RowMinHeight)
                .clickable(role = Role.DropdownList) { isExpanded = true }
                .testTag(testTag)
                .padding(horizontal = RowInset, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(label, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.weight(1f))
            Text(describe(selected), style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
            Icon(Icons.Outlined.UnfoldMore, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(18.dp))
        }
        DropdownMenu(
            expanded = isExpanded,
            onDismissRequest = { isExpanded = false },
            containerColor = HisarTheme.colors.cellBackground,
            modifier = Modifier.align(Alignment.TopEnd),
        ) {
            for (option in allOption) {
                DropdownMenuItem(
                    text = { Text(describe(option), style = HisarType.body, color = HisarTheme.colors.label) },
                    onClick = {
                        isExpanded = false
                        onSelect(option)
                    },
                    trailingIcon = if (option == selected) {
                        { Icon(Icons.Filled.Check, contentDescription = null, tint = HisarTheme.colors.tint) }
                    } else {
                        null
                    },
                    modifier = Modifier.testTag("$testTag-option-${describe(option)}"),
                )
            }
        }
    }
}

/** A row of [content] that a swipe toward the start deletes, as a Form row with onDelete. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SwipeToDeleteRow(onDelete: () -> Unit, content: @Composable () -> Unit) {
    val state = rememberSwipeToDismissBoxState()
    val currentOnDelete by rememberUpdatedState(onDelete)
    // The next row takes this one's place, so the swipe settles again once it deleted.
    LaunchedEffect(state.currentValue) {
        if (state.currentValue == SwipeToDismissBoxValue.EndToStart) {
            currentOnDelete()
            state.snapTo(SwipeToDismissBoxValue.Settled)
        }
    }
    val isSwiping = state.dismissDirection == SwipeToDismissBoxValue.EndToStart
    SwipeToDismissBox(
        state = state,
        enableDismissFromStartToEnd = false,
        backgroundContent = {
            if (isSwiping) {
                Box(
                    Modifier
                        .fillMaxSize()
                        .background(HisarTheme.colors.destructive)
                        .padding(horizontal = 20.dp),
                    contentAlignment = Alignment.CenterEnd,
                ) {
                    Text("Delete", style = HisarType.body, color = Color.White)
                }
            }
        },
    ) {
        // Only while it slides does the row need its own fill; at rest the section's rounded card shows through.
        Box(if (isSwiping) Modifier.background(HisarTheme.colors.cellBackground) else Modifier) { content() }
    }
}

private val dateFormatter = DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM)

/**
 * A date picker for a stored YYYY-MM-DD date, such as a birth date or an expiry; an imported date in another form, such
 * as 04/12/1990, or a day that does not exist, such as 2023-02-31, stays text. Empty, it offers "Add <date>", which sets
 * today on the device's calendar. A birth date cannot be in the future.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DateRow(title: String, value: String, onValueChange: (String) -> Unit, testTag: String, isFutureAllowed: Boolean) {
    val date = CalendarDate.parse(value)
    when {
        value.isEmpty() -> EditorButtonRow("Add ${title.lowercase()}", Icons.Outlined.CalendarMonth, onClick = { onValueChange(CalendarDate.formatToday()) }, testTag = "$testTag-add")
        date == null -> EditorField(value, onValueChange, title, testTag)
        else -> {
            var isPickerShown by remember { mutableStateOf(false) }
            Row(
                Modifier
                    .fillMaxWidth()
                    .heightIn(min = RowMinHeight)
                    .padding(start = RowInset, end = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(title, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.weight(1f))
                Box(
                    Modifier
                        .background(HisarTheme.colors.fill, CircleShape)
                        .clickable(role = Role.Button) { isPickerShown = true }
                        .testTag(testTag)
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                ) {
                    Text(date.format(dateFormatter), style = HisarType.body, color = HisarTheme.colors.label)
                }
                Spacer(Modifier.width(4.dp))
                RowIconButton(Icons.Outlined.RemoveCircleOutline, "Remove ${title.lowercase()}", onClick = { onValueChange("") }, tint = HisarTheme.colors.destructive)
            }
            if (isPickerShown) {
                val latest = CalendarDate.getToday()
                val pickerState = rememberDatePickerState(
                    initialSelectedDateMillis = date.toEpochMillis(),
                    selectableDates = object : SelectableDates {
                        override fun isSelectableDate(utcTimeMillis: Long): Boolean = isFutureAllowed || !utcTimeMillis.toLocalDate().isAfter(latest)

                        override fun isSelectableYear(year: Int): Boolean = isFutureAllowed || year <= latest.year
                    },
                )
                DatePickerDialog(
                    onDismissRequest = { isPickerShown = false },
                    confirmButton = {
                        ToolbarTextButton("Done", onClick = {
                            pickerState.selectedDateMillis?.let { onValueChange(CalendarDate.format(it.toLocalDate())) }
                            isPickerShown = false
                        }, testTag = "$testTag-done", isProminent = true)
                    },
                    dismissButton = { ToolbarTextButton("Cancel", onClick = { isPickerShown = false }, testTag = "$testTag-cancel") },
                    colors = DatePickerDefaults.colors(containerColor = HisarTheme.colors.cellBackground),
                ) {
                    DatePicker(
                        state = pickerState,
                        title = null,
                        showModeToggle = false,
                        colors = DatePickerDefaults.colors(
                            containerColor = HisarTheme.colors.cellBackground,
                            selectedDayContainerColor = HisarTheme.colors.tint,
                            selectedDayContentColor = HisarTheme.colors.onTint,
                            todayDateBorderColor = HisarTheme.colors.tint,
                            todayContentColor = HisarTheme.colors.tint,
                            selectedYearContainerColor = HisarTheme.colors.tint,
                            selectedYearContentColor = HisarTheme.colors.onTint,
                        ),
                    )
                }
            }
        }
    }
}

// The date picker works in UTC milliseconds; a stored date is a day without a time zone.
private fun LocalDate.toEpochMillis(): Long = atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()

private fun Long.toLocalDate(): LocalDate = Instant.ofEpochMilli(this).atZone(ZoneOffset.UTC).toLocalDate()

/** A tappable row with a leading symbol in the tint, as a Form Button with a Label. */
@Composable
fun EditorButtonRow(text: String, icon: ImageVector?, onClick: () -> Unit, testTag: String, color: Color = HisarTheme.colors.tint, enabled: Boolean = true) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = RowMinHeight)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = RowInset, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) {
            Icon(icon, contentDescription = null, tint = if (enabled) color else HisarTheme.colors.secondaryLabel, modifier = Modifier.size(22.dp))
            Spacer(Modifier.size(12.dp))
        }
        Text(text, style = HisarType.body, color = if (enabled) color else HisarTheme.colors.secondaryLabel)
    }
}

/** A confirmation as the iPhone's confirmationDialog: a title, a message, one destructive action and Cancel. */
@Composable
fun DestructiveConfirmation(title: String, message: String, action: String, onConfirm: () -> Unit, onDismiss: () -> Unit, testTag: String, dismissLabel: String = "Cancel") {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = HisarTheme.colors.cellBackground,
        title = { Text(title, style = HisarType.headline, color = HisarTheme.colors.label) },
        text = { Text(message, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel) },
        confirmButton = { ToolbarTextButton(action, onClick = onConfirm, testTag = "$testTag-confirm", isProminent = true, color = HisarTheme.colors.destructive) },
        dismissButton = { ToolbarTextButton(dismissLabel, onClick = onDismiss, testTag = "$testTag-dismiss") },
        modifier = Modifier.testTag(testTag),
    )
}
