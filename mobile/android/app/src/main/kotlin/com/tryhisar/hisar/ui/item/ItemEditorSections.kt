package com.tryhisar.hisar.ui.item

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Label
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.outlined.AutoFixHigh
import androidx.compose.material.icons.outlined.MoveUp
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.app.ItemEditorDraft
import com.tryhisar.hisar.model.DatabaseEngine
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.settings.ToggleRow
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** Which secrets the editor shows in the clear; each section has its own eye, as on the iPhone. */
data class RevealedSecrets(
    val isPassword: Boolean = false,
    val isCard: Boolean = false,
    val isDocument: Boolean = false,
    val isCredentialSecret: Boolean = false,
    val isLicenseKey: Boolean = false,
)

/** A warning line under a section, in the theme's warning color. */
@Composable
private fun Warning(text: String, testTag: String) {
    Text(text, color = HisarTheme.colors.warning, style = HisarType.footnote, modifier = Modifier.testTag(testTag))
}

@Composable
fun LoginSections(
    draft: ItemEditorDraft,
    onChange: (ItemEditorDraft) -> Unit,
    revealed: RevealedSecrets,
    onRevealedChange: (RevealedSecrets) -> Unit,
    onGeneratePassword: () -> Unit,
) {
    val item = draft.item
    FormSection(header = "Login") {
        EditorField(item.username, { value -> onChange(draft.withItem { it.copy(username = value) }) }, "Username or email", "editor-username", kind = FieldKind.PLAIN)
        RowDivider()
        EditorField(
            item.password,
            { value -> onChange(draft.withItem { it.copy(password = value) }) },
            "Password",
            "editor-password",
            kind = FieldKind.SECRET,
            isSecret = true,
            isRevealed = revealed.isPassword,
            isMonospaced = revealed.isPassword,
        ) {
            RevealButton(revealed.isPassword, "password") { onRevealedChange(revealed.copy(isPassword = !revealed.isPassword)) }
            RowIconButton(Icons.Outlined.AutoFixHigh, "Generate password", onClick = onGeneratePassword, testTag = "editor-generate-password")
        }
    }
    if (!item.isDatabase) {
        FormSection(header = "One-time password") {
            EditorField(
                item.oneTimePassword,
                { value -> onChange(draft.withItem { it.copy(oneTimePassword = value) }) },
                "Setup key or otpauth:// link",
                "editor-one-time-password",
                kind = FieldKind.SECRET,
                isMonospaced = true,
            )
        }
        FormSection(header = if (item.isServer) "Admin console" else "Websites") {
            draft.allURLText.forEachIndexed { index, text ->
                val remove = { onChange(draft.copy(allURLText = draft.allURLText.filterIndexed { other, _ -> other != index })) }
                SwipeToDeleteRow(onDelete = remove) {
                    EditorField(
                        text,
                        { value -> onChange(draft.copy(allURLText = draft.allURLText.toMutableList().apply { set(index, value) })) },
                        if (item.isServer) "https://console.example.com" else "https://example.com",
                        "editor-website-$index",
                        kind = FieldKind.URL,
                    ) {
                        RemoveRowButton("Remove website ${text.ifEmpty { "${index + 1}" }}", "editor-website-$index-remove", remove)
                    }
                }
                RowDivider()
            }
            EditorButtonRow("Add website", Icons.Filled.Add, onClick = { onChange(draft.copy(allURLText = draft.allURLText + "")) }, testTag = "editor-add-website")
        }
    }
}

@Composable
fun ServerSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit, allVaultSSHKey: List<ItemDetail>) {
    val server = draft.item.server ?: ItemServer()
    val portWarning = ItemServer.describePort(server.port)
    FormSection(header = "Server", footer = if (portWarning.isEmpty()) null else ({ Warning(portWarning, "editor-server-port-warning") })) {
        EditorField(server.host, { value -> onChange(draft.withServer { it.copy(host = value) }) }, "server.example.com", "editor-server-host", kind = FieldKind.URL)
        RowDivider()
        EditorField(server.port, { value -> onChange(draft.withServer { it.copy(port = value) }) }, "22", "editor-server-port", kind = FieldKind.NUMBER)
        RowDivider()
        val sshKeyID = server.sshKeyID
        val allChoice = buildList {
            add("" to "None")
            if (sshKeyID.isNotEmpty() && allVaultSSHKey.none { it.id == sshKeyID }) add(sshKeyID to "Missing SSH key")
            allVaultSSHKey.forEach { add(it.id to it.title) }
        }
        MenuPickerRow(
            "SSH key",
            allChoice,
            allChoice.firstOrNull { it.first == sshKeyID } ?: allChoice.first(),
            describe = { it.second },
            onSelect = { choice -> onChange(draft.withServer { it.copy(sshKeyID = choice.first) }) },
            testTag = "editor-server-ssh-key",
        )
    }
}

@Composable
fun DatabaseSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit) {
    val database = draft.item.database
    val isOtherEngine = draft.databaseEngineChoice == ItemEditorDraft.OTHER_ENGINE
    val portWarning = ItemServer.describePort(database?.port.orEmpty())
    val hasFooter = portWarning.isNotEmpty() || isOtherEngine
    FormSection(
        header = "Database",
        footer = if (!hasFooter) {
            null
        } else {
            {
                if (portWarning.isNotEmpty()) Warning(portWarning, "editor-database-port-warning")
                if (isOtherEngine) Text("Hisar builds connection strings only for the engines listed.")
            }
        },
    ) {
        val allChoice = DatabaseEngine.allKnown.map { it.id to it.name } + (ItemEditorDraft.OTHER_ENGINE to "Other")
        MenuPickerRow(
            "Engine",
            allChoice,
            allChoice.first { it.first == draft.databaseEngineChoice },
            describe = { it.second },
            onSelect = { choice -> onChange(draft.withDatabaseEngineChoice(choice.first)) },
            testTag = "editor-database-engine",
        )
        if (isOtherEngine) {
            RowDivider()
            EditorField(database?.engine.orEmpty(), { value -> onChange(draft.withDatabase { it.copy(engine = value) }) }, "DB2", "editor-database-other-engine", kind = FieldKind.PLAIN)
        }
        RowDivider()
        EditorField(database?.host.orEmpty(), { value -> onChange(draft.withDatabase { it.copy(host = value) }) }, "db.example.com", "editor-database-host", kind = FieldKind.URL)
        RowDivider()
        EditorField(database?.port.orEmpty(), { value -> onChange(draft.withDatabase { it.copy(port = value) }) }, "5432", "editor-database-port", kind = FieldKind.NUMBER)
        RowDivider()
        EditorField(database?.name.orEmpty(), { value -> onChange(draft.withDatabase { it.copy(name = value) }) }, "app", "editor-database-name", kind = FieldKind.PLAIN)
        RowDivider()
        EditorField(database?.options.orEmpty(), { value -> onChange(draft.withDatabase { it.copy(options = value) }) }, "sslmode=require", "editor-database-options", kind = FieldKind.PLAIN)
    }
}

@Composable
fun CardSections(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit, revealed: RevealedSecrets, onRevealedChange: (RevealedSecrets) -> Unit) {
    val card = draft.item.card
    val allWarning = draft.allCardWarning
    FormSection(
        header = "Card",
        footer = if (allWarning.isEmpty()) null else ({ Warning(allWarning.joinToString("\n"), "editor-card-warning") }),
    ) {
        EditorField(card?.cardholder.orEmpty(), { value -> onChange(draft.withCard { it.copy(cardholder = value) }) }, "Name on the card", "editor-cardholder", kind = FieldKind.NAME)
        RowDivider()
        EditorField(
            card?.number.orEmpty(), { value -> onChange(draft.withCard { it.copy(number = value) }) }, "Card number", "editor-card-number",
            kind = FieldKind.SECRET_NUMBER, isSecret = true, isRevealed = revealed.isCard, isMonospaced = true,
        )
        RowDivider()
        val cardType = card?.type.orEmpty()
        MenuPickerRow(
            "Type",
            listOf("") + draft.allCardType,
            cardType,
            describe = { it.ifEmpty { "Not set" } },
            onSelect = { choice -> onChange(draft.withCard { it.copy(type = choice) }) },
            testTag = "editor-card-type",
        )
        RowDivider()
        EditorField(card?.expiry.orEmpty(), { value -> onChange(draft.withCard { it.copy(expiry = value) }) }, "Expiry (MM/YY)", "editor-card-expiry", kind = FieldKind.NUMBER)
        RowDivider()
        EditorField(
            card?.securityCode.orEmpty(), { value -> onChange(draft.withCard { it.copy(securityCode = value) }) }, "Security code", "editor-card-security-code",
            kind = FieldKind.SECRET_NUMBER, isSecret = true, isRevealed = revealed.isCard, isMonospaced = true,
        )
        RowDivider()
        EditorField(
            card?.pin.orEmpty(), { value -> onChange(draft.withCard { it.copy(pin = value) }) }, "PIN", "editor-card-pin",
            kind = FieldKind.SECRET_NUMBER, isSecret = true, isRevealed = revealed.isCard, isMonospaced = true,
        )
        RowDivider()
        EditorButtonRow(
            if (revealed.isCard) "Hide number and codes" else "Show number and codes",
            if (revealed.isCard) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
            onClick = { onRevealedChange(revealed.copy(isCard = !revealed.isCard)) },
            testTag = "editor-card-reveal",
        )
    }
    AddressSection("Billing address", draft, onChange)
}

@Composable
fun IdentitySections(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit, revealed: RevealedSecrets, onRevealedChange: (RevealedSecrets) -> Unit) {
    val identity = draft.item.identity
    FormSection(header = "Name") {
        EditorField(identity?.firstName.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(firstName = value) }) }, "First name", "editor-identity-first-name", kind = FieldKind.NAME)
        RowDivider()
        EditorField(identity?.middleName.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(middleName = value) }) }, "Middle name", "editor-identity-middle-name", kind = FieldKind.NAME)
        RowDivider()
        EditorField(identity?.lastName.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(lastName = value) }) }, "Last name", "editor-identity-last-name", kind = FieldKind.NAME)
    }
    FormSection(header = "Details") {
        DateRow("Birth date", identity?.birthDate.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(birthDate = value) }) }, "editor-identity-birth-date", isFutureAllowed = false)
        RowDivider()
        EditorField(identity?.email.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(email = value) }) }, "Email", "editor-identity-email", kind = FieldKind.EMAIL)
        RowDivider()
        EditorField(identity?.phone.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(phone = value) }) }, "Phone", "editor-identity-phone", kind = FieldKind.PHONE)
    }
    AddressSection("Address", draft, onChange)
    FormSection(header = "Documents") {
        val isRevealed = revealed.isDocument
        EditorField(
            identity?.passportNumber.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(passportNumber = value) }) }, "Passport number",
            "editor-identity-passport-number", kind = FieldKind.SECRET_UPPERCASE, isSecret = true, isRevealed = isRevealed, isMonospaced = true,
        )
        RowDivider()
        EditorField(
            identity?.driversLicenseNumber.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(driversLicenseNumber = value) }) }, "Driver's license number",
            "editor-identity-drivers-license-number", kind = FieldKind.SECRET_UPPERCASE, isSecret = true, isRevealed = isRevealed, isMonospaced = true,
        )
        RowDivider()
        EditorField(
            identity?.nationalIDNumber.orEmpty(), { value -> onChange(draft.withIdentity { it.copy(nationalIDNumber = value) }) }, "National ID number",
            "editor-identity-national-id-number", kind = FieldKind.SECRET_UPPERCASE, isSecret = true, isRevealed = isRevealed, isMonospaced = true,
        )
        RowDivider()
        EditorButtonRow(
            if (isRevealed) "Hide document numbers" else "Show document numbers",
            if (isRevealed) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
            onClick = { onRevealedChange(revealed.copy(isDocument = !isRevealed)) },
            testTag = "editor-identity-reveal",
        )
    }
}

/**
 * The address fields of an address item, an identity or a card. An identity's or card's one-line address saved before it
 * had these fields shows under them, with a button that moves it into the street.
 */
@Composable
fun AddressSection(title: String, draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit) {
    val address = draft.postalAddress
    fun update(transform: (ItemAddress) -> ItemAddress) = onChange(draft.withPostalAddress(transform(address)))
    FormSection(header = title) {
        EditorField(address.street, { value -> update { it.copy(street = value) } }, "Street", "editor-address-street", maxLines = 4)
        RowDivider()
        EditorField(address.city, { value -> update { it.copy(city = value) } }, "City", "editor-address-city", kind = FieldKind.NAME)
        RowDivider()
        EditorField(address.region, { value -> update { it.copy(region = value) } }, "State or province", "editor-address-region", kind = FieldKind.NAME)
        RowDivider()
        EditorField(address.postalCode, { value -> update { it.copy(postalCode = value) } }, "Postal code", "editor-address-postal-code", kind = FieldKind.POSTAL_CODE)
        RowDivider()
        EditorField(address.country, { value -> update { it.copy(country = value) } }, "Country", "editor-address-country", kind = FieldKind.NAME)
        if (!draft.item.isAddress && draft.isOldAddressShown) {
            RowDivider()
            Column(Modifier.padding(top = 10.dp)) {
                Text("$title (old format)", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel, modifier = Modifier.padding(horizontal = 16.dp))
                EditorField(draft.oldAddress, { value -> onChange(draft.withOldAddress(value)) }, "$title (old format)", "editor-old-address", minLines = 2, maxLines = 5)
                EditorButtonRow("Use as street", Icons.Outlined.MoveUp, onClick = { onChange(draft.useOldAddressAsStreet()) }, testTag = "editor-use-as-street")
            }
        }
    }
}

@Composable
fun APICredentialSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit, revealed: RevealedSecrets, onRevealedChange: (RevealedSecrets) -> Unit) {
    val credential = draft.item.apiCredential
    val warning = draft.credentialExpiryWarning
    FormSection(header = "Credential", footer = if (warning.isEmpty()) null else ({ Warning(warning, "editor-api-credential-warning") })) {
        EditorField(credential?.service.orEmpty(), { value -> onChange(draft.withAPICredential { it.copy(service = value) }) }, "Service", "editor-api-credential-service", kind = FieldKind.NAME)
        RowDivider()
        EditorField(credential?.keyID.orEmpty(), { value -> onChange(draft.withAPICredential { it.copy(keyID = value) }) }, "Key ID", "editor-api-credential-key-id", kind = FieldKind.PLAIN, isMonospaced = true)
        RowDivider()
        EditorField(
            credential?.secret.orEmpty(), { value -> onChange(draft.withAPICredential { it.copy(secret = value) }) }, "Secret", "editor-api-credential-secret",
            kind = FieldKind.SECRET, isSecret = true, isRevealed = revealed.isCredentialSecret, isMonospaced = true,
        ) {
            RevealButton(revealed.isCredentialSecret, "secret") { onRevealedChange(revealed.copy(isCredentialSecret = !revealed.isCredentialSecret)) }
        }
        RowDivider()
        EditorField(credential?.host.orEmpty(), { value -> onChange(draft.withAPICredential { it.copy(host = value) }) }, "api.example.com", "editor-api-credential-host", kind = FieldKind.URL)
        RowDivider()
        DateRow("Expiry", credential?.expiry.orEmpty(), { value -> onChange(draft.withAPICredential { it.copy(expiry = value) }) }, "editor-api-credential-expiry", isFutureAllowed = true)
    }
}

@Composable
fun SoftwareLicenseSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit, revealed: RevealedSecrets, onRevealedChange: (RevealedSecrets) -> Unit) {
    val license = draft.item.softwareLicense
    FormSection(header = "License") {
        EditorField(license?.product.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(product = value) }) }, "Product", "editor-license-product", kind = FieldKind.NAME)
        RowDivider()
        EditorField(license?.version.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(version = value) }) }, "Version", "editor-license-version", kind = FieldKind.PLAIN)
        RowDivider()
        EditorField(
            license?.licenseKey.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(licenseKey = value) }) }, "License key", "editor-license-key",
            kind = FieldKind.SECRET, isSecret = true, isRevealed = revealed.isLicenseKey, isMonospaced = true,
        ) {
            RevealButton(revealed.isLicenseKey, "license key") { onRevealedChange(revealed.copy(isLicenseKey = !revealed.isLicenseKey)) }
        }
        RowDivider()
        EditorField(license?.licensedTo.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(licensedTo = value) }) }, "Licensed to", "editor-license-licensed-to", kind = FieldKind.NAME)
        RowDivider()
        EditorField(license?.email.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(email = value) }) }, "Email", "editor-license-email", kind = FieldKind.EMAIL)
        RowDivider()
        DateRow("Purchase date", license?.purchaseDate.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(purchaseDate = value) }) }, "editor-license-purchase-date", isFutureAllowed = false)
        RowDivider()
        EditorField(
            license?.downloadURL.orEmpty(), { value -> onChange(draft.withSoftwareLicense { it.copy(downloadURL = value) }) }, "https://example.com/download",
            "editor-license-download-url", kind = FieldKind.URL,
        )
    }
}

/** Custom fields: each with its name, its value, hidden when secret, and the Secret switch; its button or a swipe removes one. */
@Composable
fun OtherFieldsSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit) {
    val allField = draft.item.allField
    fun update(index: Int, transform: (ItemField) -> ItemField) =
        onChange(draft.withItem { item -> item.copy(allField = item.allField.toMutableList().apply { set(index, transform(get(index))) }) })
    FormSection(header = "Other fields") {
        allField.forEachIndexed { index, field ->
            val remove = { onChange(draft.withItem { item -> item.copy(allField = item.allField.filterIndexed { other, _ -> other != index }) }) }
            SwipeToDeleteRow(onDelete = remove) {
                Row(Modifier.padding(vertical = 4.dp).padding(end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        EditorField(field.name, { value -> update(index) { it.copy(name = value) } }, "Name", "editor-field-$index-name", textStyle = HisarType.footnote)
                        EditorField(
                            field.value, { value -> update(index) { it.copy(value = value) } }, "Value", "editor-field-$index-value",
                            kind = if (field.concealed) FieldKind.SECRET else FieldKind.TEXT, isSecret = field.concealed,
                        )
                        ToggleRow("Secret", field.concealed, { isOn -> update(index) { it.copy(concealed = isOn) } }, testTag = "editor-field-$index-secret")
                    }
                    RemoveRowButton("Remove field ${field.name.ifEmpty { "${index + 1}" }}", "editor-field-$index-remove", remove)
                }
            }
            RowDivider()
        }
        EditorButtonRow(
            "Add field",
            Icons.Filled.Add,
            onClick = { onChange(draft.withItem { it.copy(allField = it.allField + ItemField("", "", concealed = false)) }) },
            testTag = "editor-add-field",
        )
    }
}

/** One row per tag, removed by its button or a swipe, and a field that adds what is typed on Done; commas separate several tags. */
@Composable
fun TagSection(draft: ItemEditorDraft, onChange: (ItemEditorDraft) -> Unit) {
    val allTag = draft.item.allTag
    FormSection(
        header = "Tags",
        footer = if (allTag.isEmpty()) ({ Text("Tags group items across vaults; the Vaults screen lists them.") }) else null,
    ) {
        for (tag in allTag) {
            val remove = { onChange(draft.withItem { item -> item.copy(allTag = item.allTag - tag) }) }
            SwipeToDeleteRow(onDelete = remove) {
                Row(
                    Modifier
                        .fillMaxWidth()
                        .heightIn(min = 52.dp)
                        .padding(start = 16.dp, end = 4.dp)
                        .testTag("editor-tag-$tag"),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Start,
                ) {
                    Icon(Icons.AutoMirrored.Outlined.Label, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
                    Spacer(Modifier.size(12.dp))
                    Text(tag, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.weight(1f))
                    RemoveRowButton("Remove tag $tag", "editor-tag-$tag-remove", remove)
                }
            }
            RowDivider()
        }
        EditorField(
            draft.newTagText,
            { value -> onChange(draft.copy(newTagText = value)) },
            "Add a tag",
            "editor-new-tag",
            kind = FieldKind.PLAIN,
            imeAction = ImeAction.Done,
            onImeAction = { onChange(draft.withTagsAdded()) },
        )
    }
}
