package com.tryhisar.hisar.model

import com.tryhisar.hisar.autofill.LoginWebsite
import java.io.File
import kotlinx.io.bytestring.ByteString

/**
 * Everything the app asks of HisarKit, one method per call, with the iPhone app's names and argument order. Every method
 * blocks on the network or Argon2, so callers run it off the main thread, and every failure is a [BackendError] whose
 * message a user can read.
 */
interface VaultBackend {
    fun getStatus(): VaultStatus
    fun checkMinimumVersion(): VaultStatus
    fun signIn(server: String, email: String, secretKey: String, password: String): VaultStatus
    fun signUp(inviteLink: String, email: String, name: String, password: String): SignUpResult
    /** signUpWithoutInvite is signUp on a server whose admin opened registration: the server's address instead of an invite. */
    fun signUpWithoutInvite(server: String, email: String, name: String, password: String): SignUpResult
    fun unlock(password: String): VaultStatus
    fun unlockWithBiometric(): VaultStatus
    fun enableBiometricUnlock(password: String): VaultStatus
    fun enableNewAccountBiometricUnlock(): VaultStatus
    fun forgetNewAccountSecretKey()
    /** The Emergency Kit of the account just created, written into [directory] and recorded as saved, without the password. */
    fun exportNewAccountEmergencyKit(directory: File): EmergencyKitExport
    /** Ends the sign-up's pending record once its kit was saved, for "I saved my Emergency Kit". */
    fun acknowledgeNewAccountRecovery()
    fun disableBiometricUnlock(): VaultStatus
    fun lock(): VaultStatus
    fun signOut(): VaultStatus
    fun reportActivity()
    fun listAllVault(): List<VaultSummary>
    fun createVault(name: String): VaultSummary
    fun listAllPerson(): List<Member>
    fun invitePerson(email: String)
    fun listAllShareInvitation(): ShareInvitations
    fun acceptShareInvitation(invitationID: String)
    fun declineShareInvitation(invitationID: String)
    fun cancelShareInvitation(invitationID: String)
    fun confirmShareRecipient(invitationID: String, reviewToken: String)
    fun listAllShareAwaitingUnlock(): List<ShareAwaitingUnlock>
    fun listAllVaultMember(vaultID: String): List<Member>
    fun getVaultSharing(vaultID: String): VaultSharing
    fun shareVault(vaultID: String, email: String, role: String)
    fun changeVaultMemberRole(vaultID: String, accountID: String, role: String)
    fun removeVaultMember(vaultID: String, accountID: String)
    fun getVaultActions(vaultID: String): VaultActions
    fun renameVault(vaultID: String, name: String): VaultSummary
    fun deleteVault(vaultID: String, request: VaultDeletionRequest): VaultDeletionResult
    fun leaveVault(vaultID: String)
    fun listAllRemovedVault(): List<RemovedVaultNotice>
    fun listAllOrganization(): Organizations
    fun getOrganization(organizationID: String): Organization
    fun createOrganization(name: String): OrganizationSummary
    fun renameOrganization(organizationID: String, name: String)
    fun deleteOrganization(organizationID: String)
    fun inviteToOrganization(organizationID: String, email: String, role: String)
    fun cancelOrganizationInvitation(organizationID: String, invitationID: String)
    fun acceptOrganizationInvitation(invitationID: String)
    fun declineOrganizationInvitation(invitationID: String)
    fun setOrganizationMemberRole(organizationID: String, accountID: String, role: String)
    fun removeOrganizationMember(organizationID: String, accountID: String)
    fun leaveOrganization(organizationID: String)
    fun createTeam(organizationID: String, name: String): Team
    fun renameTeam(organizationID: String, teamID: String, name: String)
    fun deleteTeam(organizationID: String, teamID: String)
    fun addTeamMember(organizationID: String, teamID: String, accountID: String)
    fun removeTeamMember(organizationID: String, teamID: String, accountID: String)
    fun giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String)
    fun takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String)
    fun createOrganizationVault(organizationID: String, teamID: String, name: String): VaultSummary
    fun listAllWaitingVaultKey(): WaitingVaultKeys
    fun getExportOptions(): ExportOptions
    fun exportAllData(format: ExportFormat, passphrase: String, password: String, directory: File): ExportResult

    /**
     * Reads the Credential Exchange JSON another app handed over (the responseJson of Android's credential transfer) and
     * keeps the items, in memory only, until the import starts or is cancelled.
     */
    fun prepareCredentialExchangeImport(vaultID: String, credentialData: ByteArray): ImportPreview

    /**
     * Reads a Hisar export (a `.hisar`, or a plain `.json`) the file picker opened, keeping it in memory until the import
     * starts or is cancelled. An encrypted export comes back with [ImportPreview.isPassphraseRequired]; [openImportFile]
     * then opens it. A file that is not a Hisar export fails with "this is not a Hisar export".
     */
    fun prepareHisarExportImport(vaultID: String, fileName: String, content: ByteArray): ImportPreview

    /**
     * Opens the encrypted Hisar export [prepareHisarExportImport] read with its export passphrase and returns the full
     * preview. A wrong passphrase fails and leaves the export waiting for the right one.
     */
    fun openImportFile(passphrase: String): ImportPreview

    /** Shows the prepared import again for another vault, with that vault's duplicates. */
    fun previewImport(vaultID: String): ImportPreview

    /**
     * Sends one source group of the prepared import, such as a Credential Exchange collection, to vaultID; the preview's
     * vaultID is then "" when the groups go to different vaults, and startImport takes it as it is.
     */
    fun mapImportGroup(groupID: String, vaultID: String): ImportPreview
    fun startImport(vaultID: String, isSkippingDuplicate: Boolean): ImportProgress

    /** Saves the next items; an error pauses the import, and calling it again continues. */
    fun continueImport(): ImportProgress
    fun cancelImport()
    fun listAllItem(vaultID: String): List<ItemDetail>
    fun getItem(vaultID: String, itemID: String): ItemDetail
    fun saveItem(vaultID: String, item: ItemDetail): ItemDetail
    fun deleteItem(vaultID: String, itemID: String)
    fun deleteUnreadableItem(vaultID: String, itemID: String, revision: String)
    fun deleteUnreadableVault(vaultID: String, revision: String)
    fun repairVaultName(vaultID: String, revision: String, name: String): VaultSummary
    fun listAllFavoriteItem(): List<FavoriteItem>
    fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean)
    fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail
    fun describeCard(number: String, expiry: String): CardDescription
    fun getOneTimeCode(vaultID: String, itemID: String): OneTimeCode
    fun getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String): OneTimeCode
    fun copyField(vaultID: String, itemID: String, fieldName: String)
    fun generatePassword(length: Int, isDigitIncluded: Boolean, isSymbolIncluded: Boolean): String
    fun ratePassword(password: String): PasswordStrength
    fun checkMasterPassword(password: String): MasterPasswordFeedback
    fun listAllLoginForService(serviceIdentifier: String): List<LoginMatch>
    fun getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String): LoginCredential

    /** The logins linked to an Android app; a login matches only both the package and the certificate's SHA-256. */
    fun listAllLoginForAndroidApp(packageName: String, certificateSHA256: String): List<LoginMatch>
    fun getLoginForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): LoginCredential
    fun getOneTimeCodeForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): OneTimeCode

    /** The hosts of the logins' websites, for asking which of them vouch for an app in their Digital Asset Links. */
    fun listAllLoginWebsite(): List<LoginWebsite>

    fun listAllPasskey(): List<StoredPasskey>
    fun listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: List<ByteString>): List<StoredPasskey>
    fun preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): PasskeyRegistrationOptions
    fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String): PasskeyRegistrationResult
    fun assertPasskey(relyingPartyID: String, credentialID: ByteString, clientDataHash: ByteString, userVerification: String): PasskeyAssertionResult
    fun cancelPasskeyCeremonies()

    /** The relying party ID that a privileged browser's page at [origin] may use; throws when the origin may not use it. */
    fun resolvePasskeyRelyingPartyID(origin: String, relyingPartyID: String): String

    /** The relying party ID a native app names, lower-cased; throws when it is not a domain name or is a public suffix. */
    fun validatePasskeyRelyingPartyID(relyingPartyID: String): String

    /** The origin of a native app's client data: `android:apk-key-hash:` and the SHA-256 of its signing certificate. */
    fun buildAndroidAppOrigin(certificateSHA256: ByteString): String

    /** Client data for a caller that sends only a challenge: a native app, named by its origin and package. */
    fun buildPasskeyClientData(clientDataType: String, challenge: String, origin: String, androidPackageName: String): PasskeyClientData

    fun deletePasskey(vaultID: String, itemID: String, credentialID: String): ItemDetail
    fun listAllPendingApproval(): List<PhoneApproval>
    fun approveApproval(requestID: String, isRemembered: Boolean)
    fun approveApprovalWithPassword(requestID: String, password: String, isRemembered: Boolean)
    fun denyApproval(requestID: String)
    fun reconnectEvents()
    fun setAutoLock(seconds: Int): VaultStatus
    fun listAllDevice(): List<Device>
    fun revokeDevice(deviceID: String)
    fun exportEmergencyKit(password: String, directory: File): EmergencyKitExport
    fun changeMasterPassword(currentPassword: String, newPassword: String): CredentialChangeResult
    fun replaceSecretKey(password: String, directory: File): CredentialChangeResult
    fun previewAccountDeletion(): AccountDeletionPreview
    fun deleteAccountWithPlan(password: String, email: String, planVersion: String): VaultStatus
    fun listPendingRecovery(): List<PendingRecovery>
    fun resumePendingRecovery(operationID: String, password: String): SignUpResult
    fun resumePendingRecoveryWithSecretKey(operationID: String, password: String, secretKey: String, priorSecretKey: String): SignUpResult
    fun discardPreparedRecovery(operationID: String, password: String)
    fun exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: File): PendingRecoveryExport
    fun acknowledgePendingRecovery(operationID: String, password: String)
    fun dismissSignedOutReason(): VaultStatus
    fun copySecretReference(vaultID: String, itemID: String, fieldName: String): String
    fun isMacApprovalAllowed(): Boolean
    fun setMacApprovalAllowed(isAllowed: Boolean)
    fun getTheme(): VaultTheme
    fun setTheme(theme: VaultTheme): VaultTheme

    /** Turns a photo into the PNG an item's icon keeps. */
    fun prepareItemIcon(image: ByteString): ByteString

    /** A login website's icon, or null; it may ask the website. */
    fun getWebsiteIcon(url: String): ByteString?
    fun isWebsiteIconsEnabled(): Boolean
    fun setWebsiteIconsEnabled(isEnabled: Boolean)

    /** Registers the push token Firebase gave this phone ("fcm") for the signed-in account; it reaches the server and the push relay. */
    /** The Web Push subscription the UnifiedPush distributor gave, which the server then sends to. */
    fun setWebPushEndpoint(endpoint: String, p256dh: String, auth: String)
    fun removeWebPushEndpoint()
    fun setPushKindEnabled(kind: PushKind, isEnabled: Boolean)
    fun getPushSettings(): PushSettings

    /**
     * The approval preview a push carries, if the account signed it and it has not expired; null otherwise, so the
     * notification keeps the kind's own sentence. Go verifies the signature; the fields map onto a [PhoneApproval].
     */
    fun openApprovalPush(message: String): PhoneApproval?

    /** Links a login to an Android app, so AutoFill offers it there; linking it again changes nothing. */
    fun addAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail

    /** Unlinks a login from an Android app; removing a link the login does not have changes nothing. */
    fun removeAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail
}

/** A failure of a backend call, with a sentence a user can read. */
class BackendError(override val message: String, val code: String? = null) : Exception(message) {
    override fun equals(other: Any?): Boolean = other is BackendError && other.message == message && other.code == code

    override fun hashCode(): Int = 31 * message.hashCode() + (code?.hashCode() ?: 0)
    companion object {
        const val PASSKEY_VERIFICATION_FAILED = "passkey-verification-failed"
    }
}

/** The sentence to show for a failure. */
fun describeError(error: Throwable): String = error.message?.ifEmpty { null } ?: "Something went wrong."
