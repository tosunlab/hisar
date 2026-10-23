package com.tryhisar.hisar.backend

import android.os.Looper
import com.tryhisar.hisar.autofill.LoginWebsite
import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.model.CredentialChangeResult
import com.tryhisar.hisar.model.Device
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ExportOptions
import com.tryhisar.hisar.model.ExportResult
import com.tryhisar.hisar.model.FavoriteItem
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.OneTimeCode
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationSummary
import com.tryhisar.hisar.model.Organizations
import com.tryhisar.hisar.model.PasskeyAssertionResult
import com.tryhisar.hisar.model.PasskeyClientData
import com.tryhisar.hisar.model.PasskeyRegistrationOptions
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.model.PasskeyRegistrationResult
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.ShareAwaitingUnlock
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.SignUpResult
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.ThemeColorName
import com.tryhisar.hisar.model.VaultActions
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultDeletionRequest
import com.tryhisar.hisar.model.VaultDeletionResult
import com.tryhisar.hisar.model.VaultSharing
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.WaitingVaultKeys
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.platform.BiometricKeystore
import com.tryhisar.hisar.platform.ClipboardAdapter
import com.tryhisar.hisar.platform.EventRelay
import com.tryhisar.hisarkit.Hisar
import com.tryhisar.hisarkit.Hisarkit
import com.tryhisar.hisarkit.SecretKeyStore
import java.io.File
import java.time.ZoneId
import kotlin.io.encoding.Base64
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.decodeToByteString
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.nullable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

/**
 * [VaultBackend] over HisarKit, Go's core/app in the AAR. Go blocks on the network, Argon2 and biometric prompts, so every
 * call must come from a background thread.
 *
 * Starting it moves Go's local time to the device's zone (Go on Android starts in UTC) and replaces Go's trusted roots
 * with [trustedCertificates], the PEM of the device's CA store.
 */
class HisarKitBackend(
    stateDirectory: File,
    secretKeyStore: SecretKeyStore,
    biometric: BiometricKeystore?,
    clipboard: ClipboardAdapter?,
    eventListener: EventRelay?,
    appVersion: String,
    trustedCertificates: String,
) : VaultBackend {
    private val hisar: Hisar = try {
        Hisarkit.setTimeZone(ZoneId.systemDefault().id)
        Hisarkit.new_(stateDirectory.path, DEVICE_NAME, secretKeyStore, biometric, clipboard, eventListener).apply {
            setAppVersion(appVersion)
            setTrustedCertificates(trustedCertificates)
            setBiometricKeyPreparer(biometric)
        }
    } catch (error: Exception) {
        throw convertError(error)
    }

    private fun <Value> call(operation: Hisar.() -> Value): Value {
        check(Looper.myLooper() != Looper.getMainLooper()) { "HisarKit blocks, so it is never called on the main thread" }
        return try {
            hisar.operation()
        } catch (error: Exception) {
            throw convertError(error)
        }
    }

    private fun <Value> decode(serializer: KSerializer<Value>, text: String): Value = try {
        backendJson.decodeFromString(serializer, text)
    } catch (_: SerializationException) {
        throw BackendError(UNREADABLE_ANSWER)
    } catch (_: IllegalArgumentException) {
        throw BackendError(UNREADABLE_ANSWER)
    }

    private fun <Value> decodeList(serializer: KSerializer<Value>, text: String): List<Value> =
        decode(ListSerializer(serializer).nullable, text).orEmpty()

    private fun <Value> encode(serializer: KSerializer<Value>, value: Value): String = backendJson.encodeToString(serializer, value)

    private fun readStatus(text: String): VaultStatus = try {
        decode(VaultStatus.serializer(), text)
    } catch (_: BackendError) {
        null
    } ?: getStatus()

    override fun getStatus(): VaultStatus = try {
        decode(VaultStatus.serializer(), call { status() })
    } catch (_: BackendError) {
        VaultStatus.signedOut
    }

    override fun checkMinimumVersion(): VaultStatus = readStatus(call { checkMinimumVersion() })

    override fun signIn(server: String, email: String, secretKey: String, password: String): VaultStatus =
        decode(VaultStatus.serializer(), call { signIn(server, email, secretKey, password) })

    override fun signUp(inviteLink: String, email: String, name: String, password: String): SignUpResult =
        decode(SignUpResult.serializer(), call { signUp(inviteLink, email, name, password) })

    override fun unlock(password: String): VaultStatus = decode(VaultStatus.serializer(), call { unlock(password) })

    override fun unlockWithBiometric(): VaultStatus = decode(VaultStatus.serializer(), call { unlockWithBiometric() })

    override fun enableBiometricUnlock(password: String): VaultStatus =
        decode(VaultStatus.serializer(), call { enableBiometricUnlock(password) })

    override fun enableNewAccountBiometricUnlock(): VaultStatus =
        decode(VaultStatus.serializer(), call { enableNewAccountBiometricUnlock() })

    override fun forgetNewAccountSecretKey() = call { forgetNewAccountSecretKey() }

    override fun disableBiometricUnlock(): VaultStatus = decode(VaultStatus.serializer(), call { disableBiometricUnlock() })

    override fun lock(): VaultStatus = readStatus(call { lock() })

    override fun signOut(): VaultStatus = decode(VaultStatus.serializer(), call { signOut() })

    override fun reportActivity() = call { reportActivity() }

    override fun listAllVault(): List<VaultSummary> = decodeList(VaultSummary.serializer(), call { listAllVault() })

    override fun createVault(name: String): VaultSummary = decode(VaultSummary.serializer(), call { createVault(name) })

    override fun listAllPerson(): List<Member> = decodeList(Member.serializer(), call { listAllPerson() })

    override fun invitePerson(email: String) = call { invitePerson(email) }

    override fun listAllShareInvitation(): ShareInvitations =
        decode(ShareInvitations.serializer(), call { listAllShareInvitation() })

    override fun acceptShareInvitation(invitationID: String) = call { acceptShareInvitation(invitationID) }

    override fun declineShareInvitation(invitationID: String) = call { declineShareInvitation(invitationID) }

    override fun cancelShareInvitation(invitationID: String) = call { cancelShareInvitation(invitationID) }

    override fun confirmShareRecipient(invitationID: String, reviewToken: String) = call { confirmShareRecipient(invitationID, reviewToken) }

    override fun listAllShareAwaitingUnlock(): List<ShareAwaitingUnlock> =
        decodeList(ShareAwaitingUnlock.serializer(), call { listAllShareAwaitingUnlock() })

    override fun listAllVaultMember(vaultID: String): List<Member> =
        decodeList(Member.serializer(), call { listAllVaultMember(vaultID) })

    override fun getVaultSharing(vaultID: String): VaultSharing =
        decode(VaultSharing.serializer(), call { getVaultSharing(vaultID) })

    override fun shareVault(vaultID: String, email: String, role: String) = call { shareVault(vaultID, email, role) }

    override fun changeVaultMemberRole(vaultID: String, accountID: String, role: String) =
        call { changeVaultMemberRole(vaultID, accountID, role) }

    override fun removeVaultMember(vaultID: String, accountID: String) = call { removeVaultMember(vaultID, accountID) }

    override fun getVaultActions(vaultID: String): VaultActions =
        decode(VaultActions.serializer(), call { getVaultActions(vaultID) })

    override fun renameVault(vaultID: String, name: String): VaultSummary =
        decode(VaultSummary.serializer(), call { renameVault(vaultID, name) })

    override fun deleteVault(vaultID: String, request: VaultDeletionRequest): VaultDeletionResult {
        val requestJSON = encode(VaultDeletionRequest.serializer(), request)
        return decode(VaultDeletionResult.serializer(), call { deleteVault(vaultID, requestJSON) })
    }

    override fun leaveVault(vaultID: String) = call { leaveVault(vaultID) }

    override fun listAllRemovedVault(): List<RemovedVaultNotice> =
        decodeList(RemovedVaultNotice.serializer(), call { listAllRemovedVault() })

    override fun listAllOrganization(): Organizations = decode(Organizations.serializer(), call { listAllOrganization() })

    override fun getOrganization(organizationID: String): Organization =
        decode(Organization.serializer(), call { getOrganization(organizationID) })

    override fun createOrganization(name: String): OrganizationSummary =
        decode(OrganizationSummary.serializer(), call { createOrganization(name) })

    override fun renameOrganization(organizationID: String, name: String) = call { renameOrganization(organizationID, name) }

    override fun deleteOrganization(organizationID: String) = call { deleteOrganization(organizationID) }

    override fun inviteToOrganization(organizationID: String, email: String, role: String) =
        call { inviteToOrganization(organizationID, email, role) }

    override fun cancelOrganizationInvitation(organizationID: String, invitationID: String) =
        call { cancelOrganizationInvitation(organizationID, invitationID) }

    override fun acceptOrganizationInvitation(invitationID: String) = call { acceptOrganizationInvitation(invitationID) }

    override fun declineOrganizationInvitation(invitationID: String) = call { declineOrganizationInvitation(invitationID) }

    override fun setOrganizationMemberRole(organizationID: String, accountID: String, role: String) =
        call { setOrganizationMemberRole(organizationID, accountID, role) }

    override fun removeOrganizationMember(organizationID: String, accountID: String) =
        call { removeOrganizationMember(organizationID, accountID) }

    override fun leaveOrganization(organizationID: String) = call { leaveOrganization(organizationID) }

    override fun createTeam(organizationID: String, name: String): Team =
        decode(Team.serializer(), call { createTeam(organizationID, name) })

    override fun renameTeam(organizationID: String, teamID: String, name: String) = call { renameTeam(organizationID, teamID, name) }

    override fun deleteTeam(organizationID: String, teamID: String) = call { deleteTeam(organizationID, teamID) }

    override fun addTeamMember(organizationID: String, teamID: String, accountID: String) =
        call { addTeamMember(organizationID, teamID, accountID) }

    override fun removeTeamMember(organizationID: String, teamID: String, accountID: String) =
        call { removeTeamMember(organizationID, teamID, accountID) }

    override fun giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) =
        call { giveVaultToTeam(organizationID, teamID, vaultID, role) }

    override fun takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) =
        call { takeVaultFromTeam(organizationID, teamID, vaultID) }

    override fun createOrganizationVault(organizationID: String, teamID: String, name: String): VaultSummary =
        decode(VaultSummary.serializer(), call { createOrganizationVault(organizationID, teamID, name) })

    override fun listAllWaitingVaultKey(): WaitingVaultKeys =
        decode(WaitingVaultKeys.serializer(), call { listAllWaitingVaultKey() })

    override fun getExportOptions(): ExportOptions = decode(ExportOptions.serializer(), call { getExportOptions() })

    override fun exportAllData(format: ExportFormat, passphrase: String, password: String, directory: File): ExportResult {
        directory.mkdirs()
        return decode(ExportResult.serializer(), call { exportAllData(format.id, passphrase, password, directory.path) })
    }

    override fun prepareCredentialExchangeImport(vaultID: String, credentialData: ByteArray): ImportPreview =
        decode(ImportPreview.serializer(), call { prepareCredentialExchangeImport(vaultID, credentialData) })

    override fun previewImport(vaultID: String): ImportPreview = decode(ImportPreview.serializer(), call { previewImport(vaultID) })

    override fun startImport(vaultID: String, isSkippingDuplicate: Boolean): ImportProgress =
        decode(ImportProgress.serializer(), call { startImport(vaultID, isSkippingDuplicate) })

    override fun continueImport(): ImportProgress = decode(ImportProgress.serializer(), call { continueImport() })

    override fun cancelImport() = call { cancelImport() }

    override fun listAllItem(vaultID: String): List<ItemDetail> = decodeList(ItemDetail.serializer(), call { listAllItem(vaultID) })

    override fun getItem(vaultID: String, itemID: String): ItemDetail = decode(ItemDetail.serializer(), call { getItem(vaultID, itemID) })

    override fun saveItem(vaultID: String, item: ItemDetail): ItemDetail {
        val itemJSON = encode(ItemDetail.serializer(), item)
        return decode(ItemDetail.serializer(), call { saveItem(vaultID, itemJSON) })
    }

    override fun deleteItem(vaultID: String, itemID: String) = call { deleteItem(vaultID, itemID) }

    override fun listAllFavoriteItem(): List<FavoriteItem> = decodeList(FavoriteItem.serializer(), call { listAllFavoriteItem() })

    override fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean) =
        call { setItemFavorite(vaultID, itemID, isFavorite) }

    override fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail =
        decode(ItemDetail.serializer(), call { convertToSecureNote(vaultID, itemID) })

    override fun describeCard(number: String, expiry: String): CardDescription =
        decode(CardDescription.serializer(), call { describeCard(number, expiry) })

    override fun getOneTimeCode(vaultID: String, itemID: String): OneTimeCode =
        decode(OneTimeCode.serializer(), call { getOneTimeCode(vaultID, itemID) })

    override fun getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String): OneTimeCode =
        decode(OneTimeCode.serializer(), call { getOneTimeCodeForService(vaultID, itemID, serviceIdentifier) })

    override fun copyField(vaultID: String, itemID: String, fieldName: String) = call { copyField(vaultID, itemID, fieldName) }

    override fun generatePassword(length: Int, isDigitIncluded: Boolean, isSymbolIncluded: Boolean): String =
        call { generatePassword(length.toLong(), isDigitIncluded, isSymbolIncluded) }

    override fun listAllLoginForService(serviceIdentifier: String): List<LoginMatch> =
        decodeList(LoginMatch.serializer(), call { listAllLoginForService(serviceIdentifier) })

    override fun getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String): LoginCredential =
        decode(LoginCredential.serializer(), call { getLoginForService(vaultID, itemID, serviceIdentifier) })

    override fun listAllLoginForAndroidApp(packageName: String, certificateSHA256: String): List<LoginMatch> =
        decodeList(LoginMatch.serializer(), call { listAllLoginForAndroidApp(packageName, certificateSHA256) })

    override fun getLoginForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): LoginCredential =
        decode(LoginCredential.serializer(), call { getLoginForAndroidApp(vaultID, itemID, packageName, certificateSHA256) })

    override fun getOneTimeCodeForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): OneTimeCode =
        decode(OneTimeCode.serializer(), call { getOneTimeCodeForAndroidApp(vaultID, itemID, packageName, certificateSHA256) })

    override fun listAllLoginWebsite(): List<LoginWebsite> = decodeList(LoginWebsite.serializer(), call { listAllLoginWebsite() })

    override fun listAllPasskey(): List<StoredPasskey> = decodeList(StoredPasskey.serializer(), call { listAllPasskey() })

    override fun listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: List<ByteString>): List<StoredPasskey> {
        val allowedJSON = encodeAllCredentialID(allAllowedCredentialID)
        return decodeList(StoredPasskey.serializer(), call { listAllPasskeyForRelyingParty(relyingPartyID, allowedJSON) })
    }

    override fun preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): PasskeyRegistrationOptions {
        val excludedJSON = encodeAllCredentialID(allExcludedCredentialID)
        return decode(PasskeyRegistrationOptions.serializer(), call { preparePasskeyRegistration(relyingPartyID, excludedJSON) })
    }

    override fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String): PasskeyRegistrationResult {
        val registrationJSON = buildJsonObject {
            put("relyingPartyId", request.relyingPartyID)
            put("userName", request.userName)
            put("userHandle", Base64URL.encode(request.userHandle))
            put("clientDataHash", Base64URL.encode(request.clientDataHash))
            put("excludeCredentials", JsonArray(request.allExcludedCredentialID.map { JsonPrimitive(Base64URL.encode(it)) }))
            put("vaultID", vaultID)
            put("itemID", itemID)
        }.toString()
        return decode(PasskeyRegistrationResult.serializer(), call { createPasskey(registrationJSON) })
    }

    override fun assertPasskey(relyingPartyID: String, credentialID: ByteString, clientDataHash: ByteString): PasskeyAssertionResult =
        decode(
            PasskeyAssertionResult.serializer(),
            call { assertPasskey(relyingPartyID, Base64URL.encode(credentialID), Base64URL.encode(clientDataHash)) },
        )

    override fun resolvePasskeyRelyingPartyID(origin: String, relyingPartyID: String): String =
        callPackage { Hisarkit.resolvePasskeyRelyingPartyID(origin, relyingPartyID) }

    override fun validatePasskeyRelyingPartyID(relyingPartyID: String): String =
        callPackage { Hisarkit.validatePasskeyRelyingPartyID(relyingPartyID) }

    override fun buildAndroidAppOrigin(certificateSHA256: ByteString): String =
        callPackage { Hisarkit.buildAndroidAppOrigin(certificateSHA256.toByteArray()) }

    override fun buildPasskeyClientData(clientDataType: String, challenge: String, origin: String, androidPackageName: String): PasskeyClientData {
        val answer = callPackage { Hisarkit.buildPasskeyClientData(clientDataType, challenge, origin, androidPackageName) }
        val fields = decode(JsonObject.serializer(), answer)
        return PasskeyClientData(
            json = fields["clientDataJSON"]?.jsonPrimitive?.content?.let(Base64URL::decode) ?: throw BackendError(UNREADABLE_ANSWER),
            hash = fields["clientDataHash"]?.jsonPrimitive?.content?.let(Base64URL::decode) ?: throw BackendError(UNREADABLE_ANSWER),
        )
    }

    private fun <Value> callPackage(operation: () -> Value): Value = try {
        operation()
    } catch (error: Exception) {
        throw convertError(error)
    }

    override fun deletePasskey(vaultID: String, itemID: String, credentialID: String): ItemDetail =
        decode(ItemDetail.serializer(), call { deletePasskey(vaultID, itemID, credentialID) })

    override fun listAllPendingApproval(): List<PhoneApproval> =
        decodeList(PhoneApproval.serializer(), call { listAllPendingApproval() })

    override fun approveApproval(requestID: String, isRemembered: Boolean) = call { approveApproval(requestID, isRemembered) }

    override fun approveApprovalWithPassword(requestID: String, password: String, isRemembered: Boolean) =
        call { approveApprovalWithPassword(requestID, password, isRemembered) }

    override fun denyApproval(requestID: String) = call { denyApproval(requestID) }

    override fun reconnectEvents() = call { reconnectEvents() }

    override fun setAutoLock(seconds: Int): VaultStatus = decode(VaultStatus.serializer(), call { setAutoLock(seconds.toLong()) })

    override fun listAllDevice(): List<Device> = decodeList(Device.serializer(), call { listAllDevice() })

    override fun revokeDevice(deviceID: String) = call { revokeDevice(deviceID) }

    override fun exportEmergencyKit(password: String, directory: File): EmergencyKitExport =
        decode(EmergencyKitExport.serializer(), call { exportEmergencyKit(password, directory.path) })

    override fun changeMasterPassword(currentPassword: String, newPassword: String): CredentialChangeResult =
        decode(CredentialChangeResult.serializer(), call { changeMasterPassword(currentPassword, newPassword) })

    override fun replaceSecretKey(password: String, directory: File): CredentialChangeResult =
        decode(CredentialChangeResult.serializer(), call { replaceSecretKey(password, directory.path) })

    override fun previewAccountDeletion(): AccountDeletionPreview =
        decode(AccountDeletionPreview.serializer(), call { previewAccountDeletion() })

    override fun deleteAccount(password: String, email: String): VaultStatus =
        decode(VaultStatus.serializer(), call { deleteAccount(password, email) })

    override fun dismissSignedOutReason(): VaultStatus = readStatus(call { dismissSignedOutReason() })

    override fun copySecretReference(vaultID: String, itemID: String, fieldName: String): String =
        call { copySecretReference(vaultID, itemID, fieldName) }

    override fun isMacApprovalAllowed(): Boolean = call { isMacApprovalAllowed() }

    override fun setMacApprovalAllowed(isAllowed: Boolean) = call { setMacApprovalAllowed(isAllowed) }

    override fun getTheme(): VaultTheme = try {
        decode(VaultTheme.serializer(), call { getTheme() })
    } catch (_: BackendError) {
        VaultTheme.standard
    }

    override fun setTheme(theme: VaultTheme): VaultTheme {
        val appearance = backendJson.encodeToJsonElement(ThemeAppearanceName.serializer(), theme.appearance).jsonPrimitive.content
        val color = backendJson.encodeToJsonElement(ThemeColorName.serializer(), theme.color).jsonPrimitive.content
        return decode(VaultTheme.serializer(), call { setTheme(appearance, color) })
    }

    override fun prepareItemIcon(image: ByteString): ByteString {
        val encoded = call { prepareItemIcon(image.toByteArray()) }
        return decodeBase64(encoded) ?: throw BackendError(UNREADABLE_ANSWER)
    }

    override fun getWebsiteIcon(url: String): ByteString? {
        val encoded = call { getWebsiteIcon(url) }
        return if (encoded.isEmpty()) null else decodeBase64(encoded)
    }

    override fun isWebsiteIconsEnabled(): Boolean = call { isWebsiteIconsEnabled() }

    override fun setWebsiteIconsEnabled(isEnabled: Boolean) = call { setWebsiteIconsEnabled(isEnabled) }

    override fun addAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail =
        decode(ItemDetail.serializer(), call { addAndroidApp(vaultID, itemID, packageName, certificateSHA256) })

    override fun removeAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail =
        decode(ItemDetail.serializer(), call { removeAndroidApp(vaultID, itemID, packageName, certificateSHA256) })

    companion object {
        const val DEVICE_NAME = "Hisar on Android"
        private const val UNREADABLE_ANSWER = "Hisar could not read the answer from its backend."

        /** Go's sentences start in lower case and already say "biometric" on Android. */
        fun convertError(error: Throwable): BackendError {
            val message = error.message.orEmpty().ifEmpty { "Something went wrong." }
            return BackendError(message.replaceFirstChar { it.uppercaseChar() })
        }

        private fun encodeAllCredentialID(allCredentialID: List<ByteString>): String =
            JsonArray(allCredentialID.map { JsonPrimitive(Base64URL.encode(it)) }).toString()

        private fun decodeBase64(text: String): ByteString? = try {
            Base64.decodeToByteString(text)
        } catch (_: IllegalArgumentException) {
            null
        }
    }
}
