package com.tryhisar.hisar.model

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
    fun unlock(password: String): VaultStatus
    fun unlockWithBiometric(): VaultStatus
    fun enableBiometricUnlock(password: String): VaultStatus
    fun enableNewAccountBiometricUnlock(): VaultStatus
    fun forgetNewAccountSecretKey()
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
    fun listAllItem(vaultID: String): List<ItemDetail>
    fun getItem(vaultID: String, itemID: String): ItemDetail
    fun saveItem(vaultID: String, item: ItemDetail): ItemDetail
    fun deleteItem(vaultID: String, itemID: String)
    fun listAllFavoriteItem(): List<FavoriteItem>
    fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean)
    fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail
    fun describeCard(number: String, expiry: String): CardDescription
    fun getOneTimeCode(vaultID: String, itemID: String): OneTimeCode
    fun getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String): OneTimeCode
    fun copyField(vaultID: String, itemID: String, fieldName: String)
    fun generatePassword(length: Int, isDigitIncluded: Boolean, isSymbolIncluded: Boolean): String
    fun listAllLoginForService(serviceIdentifier: String): List<LoginMatch>
    fun getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String): LoginCredential
    fun listAllPasskey(): List<StoredPasskey>
    fun listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: List<ByteString>): List<StoredPasskey>
    fun preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): PasskeyRegistrationOptions
    fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String): PasskeyRegistrationResult
    fun assertPasskey(relyingPartyID: String, credentialID: ByteString, clientDataHash: ByteString): PasskeyAssertionResult
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
    fun deleteAccount(password: String, email: String): VaultStatus
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
}

/** A failure of a backend call, with a sentence a user can read. */
class BackendError(override val message: String) : Exception(message) {
    override fun equals(other: Any?): Boolean = other is BackendError && other.message == message

    override fun hashCode(): Int = message.hashCode()
}

/** The sentence to show for a failure. */
fun describeError(error: Throwable): String = error.message?.ifEmpty { null } ?: "Something went wrong."
