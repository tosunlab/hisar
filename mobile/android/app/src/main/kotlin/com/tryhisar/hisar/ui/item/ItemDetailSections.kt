package com.tryhisar.hisar.ui.item

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Notes
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.ChevronRight
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material.icons.outlined.Sell
import androidx.compose.material.icons.outlined.Share
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.VpnKey
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.APICredentialFieldName
import com.tryhisar.hisar.model.CardFieldName
import com.tryhisar.hisar.model.DatabaseFieldName
import com.tryhisar.hisar.model.IdentityFieldName
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemPasskey
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemSoftwareLicense
import com.tryhisar.hisar.model.ServerFieldName
import com.tryhisar.hisar.model.SoftwareLicenseFieldName
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.categoryDescription
import com.tryhisar.hisar.model.referenceFieldName
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.sharing.SwipeableRows
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.AppItemIcon
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** A value of a type kept in named fields; its label is also the field name copying and references use. */
private class NamedRow(val label: String, val value: String, val isConcealed: Boolean = false, val warning: String = "")

/** The sections of one item, in the iPhone app's order: header, category, other fields, websites, apps, passkeys, notes. */
@Composable
internal fun ItemDetailContent(
    model: AppModel,
    vaultID: String,
    vaultName: String,
    item: ItemDetail,
    onHeaderHeightChange: (Int) -> Unit,
    isWritable: Boolean,
    actions: ItemFieldActions,
    openSSHKey: (String) -> Unit,
    deletePasskey: (ItemPasskey) -> Unit,
    removeAndroidApp: (ItemAndroidApp) -> Unit,
    convertToSecureNote: () -> Unit,
) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    HeaderSection(model, vaultName, item, Modifier.onSizeChanged { onHeaderHeightChange(it.height) })
    when {
        item.isSSHKey -> SSHKeySection(item, actions)
        item.isSecureNote -> SecureNoteSection(item, actions)
        item.isCreditCard -> NamedSection("Card", "This card has no details yet.", buildCardRows(item.card ?: ItemCard()), actions)
        item.isIdentity -> NamedSection("Identity", "This identity has no details yet.", buildIdentityRows(item.identity ?: ItemIdentity()), actions)
        item.isAPICredential -> NamedSection("Credential", "This API credential has no details yet.", buildAPICredentialRows(item.apiCredential ?: ItemAPICredential()), actions)
        item.isAddress -> NamedSection("Address", "This address has no details yet.", buildAddressRows(item.address ?: ItemAddress()), actions)
        item.isSoftwareLicense -> SoftwareLicenseSection(item.softwareLicense ?: ItemSoftwareLicense(), actions)
        else -> {
            if (item.isServer) ServerSection(item.server ?: ItemServer(), actions, openSSHKey)
            if (item.isDatabase) NamedSection("Database", "This database has no details yet.", buildDatabaseRows(item.database ?: ItemDatabase()), actions)
            LoginSection(model, vaultID, item, actions)
            val connectionString = item.database?.connectionString.orEmpty()
            if (item.isDatabase && connectionString.isNotEmpty()) ConnectionStringSection(connectionString, actions)
        }
    }
    if (item.allField.isNotEmpty()) {
        RowsSection(header = "Other fields") {
            item.allField.forEach { field ->
                row { FieldRow(actions, field.name, field.name, field.value, field.concealed, isCustom = true) }
            }
        }
    }
    if (item.allURL.isNotEmpty()) {
        WebsiteSection(model, item)
    }
    if (item.allAndroidApp.isNotEmpty()) {
        RowsSection(
            header = if (item.allAndroidApp.size == 1) "Android app" else "Android apps",
            footer = "AutoFill offers this login in these apps. Touch and hold an app to copy its signing certificate.",
        ) {
            item.allAndroidApp.forEach { androidApp ->
                row {
                    AndroidAppRow(
                        androidApp, isWritable,
                        copyCertificate = { model.copyPlainText(androidApp.certificateSHA256, "Certificate") },
                        remove = { removeAndroidApp(androidApp) },
                    )
                }
            }
        }
    }
    if (item.allPasskey.isNotEmpty()) {
        FormSection(header = "Passkeys") {
            SwipeableRows {
                item.allPasskey.forEachIndexed { index, passkey ->
                    if (index > 0) RowDivider()
                    PasskeyRow(passkey, isWritable) { deletePasskey(passkey) }
                }
            }
        }
    }
    if (item.notes.isNotEmpty() && !item.isSecureNote) {
        FormSection(header = "Notes") {
            MenuRow(
                listOf(
                    MenuAction("Copy", Icons.Outlined.ContentCopy) { actions.copy("notes", "Notes") },
                    MenuAction("Copy secret reference", Icons.Outlined.Terminal) { actions.copyReference("notes") },
                ),
                onClick = { actions.copy("notes", "Notes") },
                onClickLabel = "Copy notes",
            ) {
                FormRow { Text(item.notes, style = HisarType.body, color = HisarTheme.colors.label) }
            }
        }
    }
    if (isWritable && item.canConvertToSecureNote) {
        FormSection(footer = { Text("This login has no username, password or website. As a secure note, it opens straight to its notes and fields.") }) {
            FormButtonRow(
                "Convert to secure note",
                onClick = convertToSecureNote,
                testTag = "convert-to-secure-note",
                icon = Icons.AutoMirrored.Outlined.Notes,
                color = HisarTheme.colors.tint,
                enabled = !isBusy,
            )
        }
    }
    item.referenceFieldName?.let { referenceField ->
        FormSection(footer = { Text("A hisar:// reference to the $referenceField, for hisar run and hisar read. Touch and hold a field for its own reference.") }) {
            FormButtonRow("Copy secret reference", onClick = { actions.copyReference(referenceField) }, testTag = "copy-secret-reference", icon = Icons.Outlined.Terminal)
        }
    }
}

/** Collects a section's rows, so [RowsSection] can put a divider between each two. */
private class RowsScope {
    val allRow = mutableListOf<@Composable () -> Unit>()

    fun row(content: @Composable () -> Unit) {
        allRow += content
    }
}

@Composable
private fun RowsSection(header: String? = null, footer: String? = null, rows: RowsScope.() -> Unit) {
    val allRow = RowsScope().apply(rows).allRow
    if (allRow.isEmpty()) return
    FormSection(header = header, footer = footer?.let { text -> { Text(text) } }) {
        DividedRows(allRow)
    }
}

@Composable
private fun ColumnScope.DividedRows(allRow: List<@Composable () -> Unit>) {
    allRow.forEachIndexed { index, row ->
        if (index > 0) RowDivider()
        row()
    }
}

/** A field with copy and copy reference; a concealed one is hidden until revealed. */
@Composable
private fun FieldRow(actions: ItemFieldActions, fieldName: String, label: String, value: String, isConcealed: Boolean, warning: String = "", isCustom: Boolean = false) {
    SecretFieldRow(
        label = label,
        value = value,
        isConcealed = isConcealed,
        isRevealed = !isConcealed || actions.isRevealed(fieldName),
        toggleReveal = if (isConcealed) ({ actions.toggleReveal(fieldName) }) else null,
        copy = { actions.copy(fieldName, label) },
        copyReference = { actions.copyReference(fieldName) },
        warning = warning,
        copyErrorMessage = actions.errorMessage,
        actionLabel = if (isCustom || label.take(2).all(Char::isUpperCase)) label else label.replaceFirstChar { it.lowercase() },
    )
}

@Composable
private fun HeaderSection(model: AppModel, vaultName: String, item: ItemDetail, modifier: Modifier) {
    ClearSection(modifier) {
        Row(
            Modifier.padding(horizontal = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            AppItemIcon(model, item, size = 56.dp)
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(item.title, style = HisarType.title2, color = HisarTheme.colors.label)
                Text(
                    "$vaultName · ${item.categoryDescription}",
                    style = HisarType.subheadline,
                    color = HisarTheme.colors.secondaryLabel,
                    modifier = Modifier.testTag("item-subtitle"),
                )
            }
        }
        if (item.allTag.isNotEmpty()) {
            Row(
                Modifier
                    .padding(top = 12.dp)
                    .horizontalScroll(rememberScrollState())
                    .padding(horizontal = 4.dp, vertical = 2.dp)
                    .testTag("item-tags"),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                item.allTag.forEach { tag -> TagChip(tag) }
            }
        }
    }
}

@Composable
private fun TagChip(tag: String) {
    Row(
        Modifier
            .background(HisarTheme.colors.fill, CircleShape)
            .padding(horizontal = 10.dp, vertical = 5.dp)
            .testTag("item-tag-$tag"),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Outlined.Sell, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(13.dp))
        Text(tag, style = HisarType.footnote.copy(fontSize = HisarType.footnote.fontSize * 0.92f, fontWeight = FontWeight.Medium), color = HisarTheme.colors.secondaryLabel)
    }
}

@Composable
private fun LoginSection(model: AppModel, vaultID: String, item: ItemDetail, actions: ItemFieldActions) {
    RowsSection {
        if (item.username.isNotEmpty()) row { FieldRow(actions, "username", "Username", item.username, isConcealed = false) }
        if (item.password.isNotEmpty()) row { FieldRow(actions, "password", "Password", item.password, isConcealed = true) }
        if (item.hasOneTimePassword) {
            row {
                OneTimeCodeRow(
                    model, vaultID, item.id,
                    copy = { actions.copy("one-time code", "One-time code") },
                    copyReference = { actions.copyReference("one-time code") },
                )
            }
        }
    }
}

@Composable
private fun SSHKeySection(item: ItemDetail, actions: ItemFieldActions) {
    FormSection(header = "SSH key") {
        LabeledRow("Key type", item.keyType)
        RowDivider()
        FieldRow(actions, "fingerprint", "Fingerprint", item.fingerprint, isConcealed = false)
        RowDivider()
        FieldRow(actions, "public key", "Public key", item.publicKey, isConcealed = false)
    }
}

@Composable
private fun SecureNoteSection(item: ItemDetail, actions: ItemFieldActions) {
    FormSection(header = "Note") {
        if (item.notes.isEmpty()) {
            FormRow { Text("This note is empty.", style = HisarType.body, color = HisarTheme.colors.secondaryLabel) }
        } else {
            MenuRow(emptyList(), onClick = { actions.copy("notes", "Notes") }, onClickLabel = "Copy notes") {
                FormRow { Text(item.notes, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.testTag("secure-note-notes")) }
            }
            RowDivider()
            FormButtonRow("Copy notes", onClick = { actions.copy("notes", "Notes") }, testTag = "copy-notes", icon = Icons.Outlined.ContentCopy, color = HisarTheme.colors.tint)
        }
    }
}

/** The values of a type kept in named fields; empty ones are left out, and a section with none says so. */
@Composable
private fun NamedSection(title: String, emptyText: String, allRow: List<NamedRow>, actions: ItemFieldActions) {
    val allShownRow = allRow.filter { it.value.isNotEmpty() }
    FormSection(header = title) {
        if (allShownRow.isEmpty()) {
            FormRow { Text(emptyText, style = HisarType.body, color = HisarTheme.colors.secondaryLabel) }
        }
        DividedRows(allShownRow.map { row -> { FieldRow(actions, row.label, row.label, row.value, row.isConcealed, row.warning) } })
    }
}

private fun buildAddressRows(address: ItemAddress): List<NamedRow> = address.allNamedValue.map { (name, value) -> NamedRow(name, value) }

private fun buildCardRows(card: ItemCard): List<NamedRow> = listOf(
    NamedRow(CardFieldName.CARDHOLDER, card.cardholder),
    NamedRow(CardFieldName.TYPE, card.type),
    NamedRow(CardFieldName.NUMBER, card.number, isConcealed = true, warning = card.numberWarning),
    NamedRow(CardFieldName.EXPIRY, card.expiry, warning = card.expiryWarning),
    NamedRow(CardFieldName.SECURITY_CODE, card.securityCode, isConcealed = true),
    NamedRow(CardFieldName.PIN, card.pin, isConcealed = true),
) + buildAddressRows(card.postalAddress) + NamedRow(CardFieldName.BILLING_ADDRESS, card.billingAddress)

private fun buildIdentityRows(identity: ItemIdentity): List<NamedRow> = listOf(
    NamedRow(IdentityFieldName.FIRST_NAME, identity.firstName),
    NamedRow(IdentityFieldName.MIDDLE_NAME, identity.middleName),
    NamedRow(IdentityFieldName.LAST_NAME, identity.lastName),
    NamedRow(IdentityFieldName.BIRTH_DATE, identity.birthDate),
    NamedRow(IdentityFieldName.EMAIL, identity.email),
    NamedRow(IdentityFieldName.PHONE, identity.phone),
) + buildAddressRows(identity.postalAddress) + listOf(
    NamedRow(IdentityFieldName.ADDRESS, identity.address),
    NamedRow(IdentityFieldName.PASSPORT_NUMBER, identity.passportNumber, isConcealed = true),
    NamedRow(IdentityFieldName.DRIVERS_LICENSE_NUMBER, identity.driversLicenseNumber, isConcealed = true),
    NamedRow(IdentityFieldName.NATIONAL_ID_NUMBER, identity.nationalIDNumber, isConcealed = true),
)

private fun buildAPICredentialRows(credential: ItemAPICredential): List<NamedRow> = listOf(
    NamedRow(APICredentialFieldName.SERVICE, credential.service),
    NamedRow(APICredentialFieldName.KEY_ID, credential.keyID),
    NamedRow(APICredentialFieldName.SECRET, credential.secret, isConcealed = true),
    NamedRow(APICredentialFieldName.HOST, credential.host),
    NamedRow(APICredentialFieldName.EXPIRY, credential.expiry, warning = credential.expiryWarning),
)

private fun buildDatabaseRows(database: ItemDatabase): List<NamedRow> = listOf(
    NamedRow(DatabaseFieldName.ENGINE, database.engineName),
    NamedRow(DatabaseFieldName.HOST, database.host),
    NamedRow(DatabaseFieldName.PORT, database.port, warning = database.portWarning),
    NamedRow(DatabaseFieldName.NAME, database.name),
    NamedRow(DatabaseFieldName.OPTIONS, database.options),
)

/** A license's values, and a link that opens an http or https download URL in the browser. */
@Composable
private fun SoftwareLicenseSection(license: ItemSoftwareLicense, actions: ItemFieldActions) {
    val allRow = listOf(
        NamedRow(SoftwareLicenseFieldName.PRODUCT, license.product),
        NamedRow(SoftwareLicenseFieldName.VERSION, license.version),
        NamedRow(SoftwareLicenseFieldName.LICENSE_KEY, license.licenseKey, isConcealed = true),
        NamedRow(SoftwareLicenseFieldName.LICENSED_TO, license.licensedTo),
        NamedRow(SoftwareLicenseFieldName.EMAIL, license.email),
        NamedRow(SoftwareLicenseFieldName.PURCHASE_DATE, license.purchaseDate),
        NamedRow(SoftwareLicenseFieldName.DOWNLOAD_URL, license.downloadURL),
    )
    NamedSection("License", "This software license has no details yet.", allRow, actions)
    VaultLogic.convertToOpenableURL(license.downloadURL)?.let { downloadURL ->
        val context = LocalContext.current
        FormSection {
            LinkRow("Open download page", Icons.Outlined.Language, "open-download-url") { openURL(context, downloadURL.toString()) }
        }
    }
}

/** A server's host and port, and its SSH key, which opens the key item. */
@Composable
private fun ServerSection(server: ItemServer, actions: ItemFieldActions, openSSHKey: (String) -> Unit) {
    RowsSection(header = "Server") {
        if (server.host.isNotEmpty()) row { FieldRow(actions, ServerFieldName.HOST, ServerFieldName.HOST, server.address, isConcealed = false) }
        if (server.port.isNotEmpty()) row { FieldRow(actions, ServerFieldName.PORT, ServerFieldName.PORT, server.port, isConcealed = false, warning = server.portWarning) }
        if (server.sshKeyID.isNotEmpty()) {
            row {
                if (server.sshKeyTitle.isEmpty()) {
                    Column(Modifier.testTag("server-ssh-key")) { LabeledRow(ServerFieldName.SSH_KEY, "Missing SSH key") }
                } else {
                    SSHKeyLinkRow(server) { openSSHKey(server.sshKeyID) }
                }
            }
        }
    }
}

@Composable
private fun SSHKeyLinkRow(server: ItemServer, open: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(role = Role.Button, onClick = open)
            .testTag("server-ssh-key")
            .padding(horizontal = 16.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(ServerFieldName.SSH_KEY, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.VpnKey, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(20.dp))
                Text(server.sshKeyTitle, style = HisarType.body, color = HisarTheme.colors.label)
            }
            Text(
                server.sshKeyFingerprint,
                style = HisarType.footnote.copy(fontFamily = HisarType.bodyMonospaced.fontFamily),
                color = HisarTheme.colors.secondaryLabel,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(Icons.Outlined.ChevronRight, contentDescription = null, tint = HisarTheme.colors.secondaryLabel)
    }
}

/** The connection string the backend builds from the database's values and password, hidden until revealed. */
@Composable
private fun ConnectionStringSection(connectionString: String, actions: ItemFieldActions) {
    FormSection(footer = { Text("Built from the fields above, so it always has the current password.") }) {
        Column(Modifier.testTag("database-connection-string")) {
            FieldRow(actions, DatabaseFieldName.CONNECTION_STRING, "Connection string", connectionString, isConcealed = true)
        }
    }
}

/**
 * A tap copies a website and its button opens it in the browser; touch and hold copies or shares it. A server calls them
 * its admin console.
 */
@Composable
private fun WebsiteSection(model: AppModel, item: ItemDetail) {
    val context = LocalContext.current
    val header = when {
        item.isServer -> "Admin console"
        item.allURL.size == 1 -> "Website"
        else -> "Websites"
    }
    RowsSection(header = header) {
        item.allURL.forEach { url ->
            row {
                val openableURL = VaultLogic.convertToOpenableURL(url)?.toString()
                val copy = { model.copyText(url, "Website") }
                MenuRow(
                    listOfNotNull(
                        MenuAction("Copy", Icons.Outlined.ContentCopy, copy),
                        openableURL?.let { MenuAction("Share", Icons.Outlined.Share) { shareURL(context, it) } },
                    ),
                    onClick = copy,
                    onClickLabel = "Copy website",
                    modifier = Modifier.testTag("website-$url"),
                ) {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .heightIn(min = 52.dp)
                            .padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Icon(Icons.Outlined.Language, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(22.dp))
                        Spacer(Modifier.size(14.dp))
                        Text(url, style = HisarType.body, color = HisarTheme.colors.label, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                        if (openableURL != null) {
                            FieldIconButton(Icons.AutoMirrored.Outlined.OpenInNew, "Open website", "open-website-$url", { openURL(context, openableURL) })
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun LinkRow(title: String, icon: ImageVector, testTag: String, open: () -> Unit) {
    Column(Modifier.clickable(role = Role.Button, onClick = open).testTag(testTag)) { LinkLabel(title, icon) }
}

@Composable
private fun LinkLabel(title: String, icon: ImageVector) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
        Spacer(Modifier.size(14.dp))
        Text(title, style = HisarType.body, color = HisarTheme.colors.tint, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/** An app the login fills into; touch and hold copies its signing certificate's SHA-256, which the row leaves out. */
@Composable
private fun AndroidAppRow(androidApp: ItemAndroidApp, isWritable: Boolean, copyCertificate: () -> Unit, remove: () -> Unit) {
    MenuRow(listOf(MenuAction("Copy certificate", Icons.Outlined.ContentCopy, copyCertificate))) {
        IconRow(Icons.Outlined.PhoneAndroid, "android-app-${androidApp.packageName}", trailing = {
            if (isWritable) {
                FieldIconButton(Icons.Outlined.Delete, "Remove ${androidApp.packageName}", "remove-android-app-${androidApp.packageName}", remove, tint = HisarTheme.colors.destructive)
            }
        }) {
            Text(androidApp.packageName, style = HisarType.body, color = HisarTheme.colors.label)
        }
    }
}

@Composable
private fun PasskeyRow(passkey: ItemPasskey, isWritable: Boolean, delete: () -> Unit) {
    val row = @Composable {
        IconRow(Icons.Outlined.Key, "passkey-${passkey.relyingPartyID}", trailing = {
            if (isWritable) {
                FieldIconButton(Icons.Outlined.Delete, "Delete passkey for ${passkey.relyingPartyID}", "delete-passkey-${passkey.relyingPartyID}", delete, tint = HisarTheme.colors.destructive)
            }
        }) {
            Text(passkey.relyingPartyID, style = HisarType.body, color = HisarTheme.colors.label)
            Text(passkey.userName.ifEmpty { passkey.userDisplayName }, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
            passkey.createdDate?.let { createdDate ->
                val day = createdDate.atZone(ZoneId.systemDefault()).toLocalDate()
                Text("Created ${DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).format(day)}", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
            }
        }
    }
    if (isWritable) {
        SwipeActionRow(
            "Delete", Icons.Outlined.Delete, "swipe-delete-passkey-${passkey.relyingPartyID}", isLeading = false,
            onAction = delete, actionColor = HisarTheme.colors.destructive, content = row,
        )
    } else {
        row()
    }
}

/** A row with a leading symbol, as SwiftUI's Label, and an optional trailing button. */
@Composable
private fun IconRow(icon: ImageVector, testTag: String, trailing: @Composable () -> Unit, content: @Composable ColumnScope.() -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .background(HisarTheme.colors.cellBackground)
            .padding(start = 16.dp, end = 8.dp, top = 11.dp, bottom = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
        Spacer(Modifier.size(14.dp))
        Column(Modifier.weight(1f).testTag(testTag), verticalArrangement = Arrangement.spacedBy(2.dp), content = content)
        trailing()
    }
}

private fun openURL(context: Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    } catch (_: ActivityNotFoundException) {
        // No browser is installed; the row stays as it is, as a link iOS cannot open.
    }
}

private fun shareURL(context: Context, url: String) {
    val intent = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, url)
    context.startActivity(Intent.createChooser(intent, null))
}
