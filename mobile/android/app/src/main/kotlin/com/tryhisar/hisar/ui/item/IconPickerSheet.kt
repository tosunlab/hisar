package com.tryhisar.hisar.ui.item

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.outlined.Photo
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.ItemEditorModel
import com.tryhisar.hisar.model.ChosenIcon
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.AppItemIcon
import com.tryhisar.hisar.ui.vaults.ItemIcon
import com.tryhisar.hisar.ui.vaults.ItemIconStyle
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.io.bytestring.ByteString

private const val COLUMN_COUNT = 6

/**
 * Chooses an item's icon: Automatic (null), a preset in a color, or a photo, which the backend turns into the small image
 * the item keeps. The choice is saved with the item.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun IconPickerSheet(model: AppModel, item: ItemDetail, onIconChange: (ChosenIcon?) -> Unit, onDismiss: () -> Unit) {
    val icon = item.icon
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val color = icon?.color.orEmpty()
    var isPreparing by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    fun preparePhoto(uri: Uri) {
        isPreparing = true
        scope.launch {
            val photo = withContext(Dispatchers.IO) {
                runCatching { context.contentResolver.openInputStream(uri)?.use { ByteString(it.readBytes()) } }.getOrNull()
            }
            if (photo == null) {
                model.errorMessage.value = ItemEditorModel.UNREADABLE_PHOTO_MESSAGE
            } else {
                model.itemEditor.prepareItemIcon(photo, IconPhotoConverter::convert)?.let { onIconChange(ChosenIcon(image = it)) }
            }
            isPreparing = false
        }
    }

    val photoPicker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> uri?.let(::preparePhoto) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = HisarTheme.colors.groupedBackground,
        dragHandle = { BottomSheetDefaults.DragHandle() },
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 12.dp),
        ) {
            Text("Icon", style = HisarType.headline, color = HisarTheme.colors.label, textAlign = TextAlign.Center, modifier = Modifier.align(Alignment.Center))
            Row(Modifier.align(Alignment.CenterEnd)) {
                GlassTextButton("Done", onClick = onDismiss, testTag = "icon-done", isProminent = true)
            }
        }
        Column(
            Modifier
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp)
                .padding(bottom = 32.dp),
        ) {
            errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
            FormSection {
                Row(
                    Modifier
                        .fillMaxWidth()
                        .heightIn(min = 52.dp)
                        .clickable(role = Role.Button) {
                            model.itemEditor.clearError()
                            onIconChange(null)
                        }
                        .testTag("icon-automatic")
                        .padding(horizontal = 16.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    AppItemIcon(model, item.copy(icon = null), size = 36.dp)
                    Spacer(Modifier.size(12.dp))
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text("Automatic", style = HisarType.body, color = HisarTheme.colors.label)
                        Text(
                            if (item.isLogin && item.allURL.isNotEmpty()) "The website's icon, or the initial" else "The initial or the item type's symbol",
                            style = HisarType.footnote,
                            color = HisarTheme.colors.secondaryLabel,
                        )
                    }
                    if (icon == null) Icon(Icons.Filled.Check, contentDescription = null, tint = HisarTheme.colors.tint)
                }
                RowDivider()
                EditorButtonRow(
                    if (isPreparing) "Preparing…" else "Choose photo",
                    Icons.Outlined.Photo,
                    onClick = { photoPicker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)) },
                    testTag = "icon-photo",
                    enabled = !isPreparing,
                )
            }
            FormSection(header = "Icons") {
                Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    for (row in ChosenIcon.allPreset.chunked(COLUMN_COUNT)) {
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                            for (preset in row) {
                                val isChosen = icon?.image == null && icon?.preset == preset
                                Box(
                                    Modifier
                                        .border(2.dp, if (isChosen) HisarTheme.colors.tint else Color.Transparent, RoundedCornerShape(13.dp))
                                        .clickable(role = Role.Button) {
                                            model.itemEditor.clearError()
                                            onIconChange(ChosenIcon(preset = preset, color = color))
                                        }
                                        .semantics {
                                            contentDescription = preset.replaceFirstChar { it.uppercase() }
                                            selected = isChosen
                                        }
                                        .testTag("icon-preset-$preset")
                                        .padding(3.dp),
                                ) {
                                    ItemIcon(item.copy(icon = ChosenIcon(preset = preset, color = color)), size = 40.dp)
                                }
                            }
                        }
                    }
                }
            }
            val isPhoto = icon?.image != null
            FormSection(header = "Color", footer = if (isPhoto) ({ Text("Choose an icon to color it.") }) else null) {
                Row(
                    Modifier
                        .horizontalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp, vertical = 12.dp)
                        .alpha(if (isPhoto) 0.4f else 1f),
                    horizontalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    for (hue in listOf("") + ChosenIcon.allColor) {
                        val isChosen = !isPhoto && color == hue
                        Box(
                            Modifier
                                .size(48.dp)
                                .clickable(enabled = !isPhoto, role = Role.Button) {
                                    val colored = ChosenIcon.applyColor(icon, hue, item.category)
                                    if (colored != icon) onIconChange(colored)
                                }
                                .semantics {
                                    contentDescription = if (hue.isEmpty()) "No color" else hue.replaceFirstChar { it.uppercase() }
                                    selected = isChosen
                                }
                                .testTag("icon-color-${hue.ifEmpty { "none" }}")
                                .padding(10.dp)
                                .background(ItemIconStyle.getColor(hue, HisarTheme.colors.isDark) ?: HisarTheme.colors.fill, CircleShape)
                                .border(2.5.dp, if (isChosen) HisarTheme.colors.tint else Color.Transparent, CircleShape),
                        )
                    }
                }
            }
        }
    }
}
