package com.tryhisar.hisar.model

import com.tryhisar.hisar.autofill.LoginWebsite
import java.io.File
import java.util.UUID
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer

/** One source group of the pending import: the titles of its logins and the vault they go to. */
data class FakeImportGroup(val id: String, val name: String, val allTitle: List<String>, var vaultID: String)

/** An in-memory VaultBackend that answers as core/app does, the port of the iPhone tests' FakeBackend. */
open class FakeBackend : VaultBackend {
    private val callLock = Any()
    private val approvalLock = Any()

    // The JVM names keep these properties apart from VaultBackend's getStatus, getTheme and getExportOptions.
    @get:JvmName("statusValue") @set:JvmName("statusValue")
    var status = VaultStatus.signedOut
    var allVault: MutableList<VaultSummary> = mutableListOf()
    var allItemByVaultID: MutableMap<String, MutableList<ItemDetail>> = mutableMapOf()
    /** Starred items as "vaultID/itemID"; the account keeps them apart from the items. */
    var allFavoriteRecordID: MutableSet<String> = mutableSetOf()
    var favoriteErrorMessage: String? = null
    var isBiometricAvailable = true
    var isBiometricSucceeding = true
    var verifyPasskey: (String) -> Boolean = { true }
    override fun cancelPasskeyCeremonies() {}
    var isOffline = false

    /** While set, deleting the account succeeds on the server and then reports that the local cache stayed. */
    var isCacheRemovalFailing = false
    val allCopiedField = mutableListOf<String>()
    val allCallName = mutableListOf<String>()
    var nextItemNumber = 1
    var allPasskey: MutableList<StoredPasskey> = mutableListOf()
    /**
     * Runs when a refresh lists the passkeys, its last call, after the unlock check: a test can sign out or lock while a
     * refresh that listed everything is still in flight.
     */
    var onListAllPasskey: (() -> Unit)? = null
    /** Runs when the open requests have been read and before they are returned: a test can hold an older listing while a newer one runs. */
    @Volatile var onListAllPendingApproval: (() -> Unit)? = null
    var allMember: MutableList<Member> = mutableListOf()
    var allVaultMemberByVaultID: MutableMap<String, MutableList<Member>> = mutableMapOf()
    val allInvitedEmail = mutableListOf<String>()
    var shareInvitations = ShareInvitations()
    var allShareAwaitingUnlock: List<ShareAwaitingUnlock> = emptyList()
    var isShareAwaitingUnlockFailing = false
    val allAnsweredInvitation = mutableListOf<String>()
    var allPendingApproval: MutableList<PhoneApproval> = mutableListOf()
    val allApprovalAnswer = mutableListOf<String>()
    var allDevice: MutableList<Device> = mutableListOf()
    /** While set, listing the devices fails with this sentence. */
    var listAllDeviceErrorMessage: String? = null
    var isMacApprovalAllowedValue = true
    var isWebsiteIconsEnabledValue = true
    /** The Web Push endpoint the app registered; null before it has one and after it removed it. */
    var webPushEndpoint: String? = null
    var pushSettingsValue = PushSettings(isServerSending = true)
    var websiteIconByURL: MutableMap<String, ByteString> = mutableMapOf()
    var preparedIcon: ByteString? = null
    @get:JvmName("themeValue") @set:JvmName("themeValue")
    var theme = VaultTheme.standard
    var isThemeSaveFailing = false
    val allCopiedReference = mutableListOf<String>()
    var allPendingRemovedVault: List<RemovedVaultNotice> = emptyList()
    val allDeletionRequest = mutableListOf<VaultDeletionRequest>()
    @get:JvmName("exportOptionsValue") @set:JvmName("exportOptionsValue")
    var exportOptions = ExportOptions(isBiometricAvailable = true, minPassphraseLength = 10)
    var currentPassword = PASSWORD
    var currentSecretKey = SECRET_KEY
    var deletionPreview = AccountDeletionPreview(email = "alice@example.com", planVersion = "reviewed-plan")
    var isAccountDeleted = false
    var allPendingRecovery: MutableList<PendingRecovery> = mutableListOf()
    var pendingRecoveryPassword = PASSWORD
    var pendingRecoverySecretKey = SECRET_KEY
    var isPendingRecoveryFactorAvailable = true
    var pendingSignUpResult: SignUpResult? = null
    /** Whether the backend still keeps the new account's Secret Key, the proof Save your Secret Key acts on. */
    var isNewAccountSecretKeyKept = false
    /** The server signUpWithoutInvite was last asked to create an account on; null until it is called. */
    var signUpWithoutInviteServer: String? = null
    /** While set, signUpWithoutInvite fails with this sentence: the server's admin has not opened registration. */
    var signUpWithoutInviteErrorMessage: String? = null
    var pendingCredentialChangeResult: CredentialChangeResult? = null
    /** The organizations the user is in, as GetOrganization answers for each; a refusal is the server's next sentence. */
    var allOrganization: MutableList<Organization> = mutableListOf()
    var allOrganizationInvitation: MutableList<OrganizationInvitation> = mutableListOf()
    var waitingVaultKeys = WaitingVaultKeys()
    var organizationRefusal: String? = null
    /** While set, listing the organizations or reading one fails with this sentence. */
    var listAllOrganizationErrorMessage: String? = null
    var getOrganizationErrorMessage: String? = null
    /** The server's answer to the next version check; null means it accepts this version. */
    var serverUpdateRequirement: UpdateRequirement? = null
    /** The import prepareCredentialExchangeImport keeps, as the logins of each source group it would save; null when none is pending. */
    var allPendingImportGroup: List<FakeImportGroup>? = null
    val allPendingImportTitle: List<String>? get() = allPendingImportGroup?.flatMap { it.allTitle }
    var pendingImportVaultID = ""
    var allQueuedImport: List<Pair<String, String>> = emptyList()
    var importSavedCount = 0
    var importBatchSize = 2
    val allImportedCredentialData = mutableListOf<ByteArray>()
    /** While set, the next continueImport fails with this sentence and saves nothing. */
    var continueImportErrorMessage: String? = null
    /** Whether the next prepareHisarExportImport reads an encrypted export, which asks for its passphrase first. */
    var isHisarExportEncrypted = true
    /** The logins a Hisar export would import, as one source group named for its vault. */
    var allHisarExportTitle = listOf("GitHub", "Figma")
    /** The account the Hisar export came from, shown in the preview. */
    var hisarExportFrom = "alice@example.com"
    /** When the Hisar export was made, shown in the preview. */
    var hisarExportAt = "2026-10-20T12:00:00Z"
    /** A warning the Hisar export carries, e.g. a cached or partial copy; empty when none. */
    var hisarExportNotice = ""
    /** Set while an encrypted Hisar export waits for its passphrase: the vaultID and file name openImportFile opens. */
    private var waitingHisarExport: Pair<String, String>? = null

    fun <Value> withApprovalLock(operation: () -> Value): Value = synchronized(approvalLock, operation)

    private fun record(name: String) {
        synchronized(callLock) { allCallName += name }
    }

    fun countCall(name: String): Int = synchronized(callLock) { allCallName.count { it == name } }

    fun seedSignedIn(state: String = VaultStatus.STATE_UNLOCKED) {
        status = VaultStatus(
            state = state, server = "http://127.0.0.1:18080", email = "alice@example.com", name = "Alice",
            biometricAvailable = isBiometricAvailable, biometricEnabled = false, biometricReady = false, isOffline = isOffline,
        )
        allVault = mutableListOf(
            VaultSummary(id = "family", name = "Family", type = "shared", role = "editor", itemCount = 1),
            VaultSummary(id = "personal", name = "Personal", type = "personal", role = "owner", itemCount = 2),
        )
        allItemByVaultID = mutableMapOf(
            "personal" to mutableListOf(
                ItemDetail(
                    id = "github", revision = "r1", category = "login", title = "GitHub", username = "alice", allURL = listOf("https://github.com"),
                    password = "github-password", notes = "work account", allField = listOf(ItemField("PIN", "1234", concealed = true)),
                    oneTimePassword = ONE_TIME_PASSWORD, hasOneTimePassword = true,
                    allPasskey = listOf(ItemPasskey(gitHubPasskey.credentialID, "github.com", "alice", "Alice", "2026-10-09T12:00:00Z")),
                ),
                ItemDetail(
                    id = "bank", revision = "r1", category = "login", title = "Bank", username = "alice@example.com",
                    allURL = listOf("bank.example.com"), password = "bank-password",
                ),
            ),
            "family" to mutableListOf(
                ItemDetail(
                    id = "netflix", revision = "r1", category = "login", title = "Netflix", username = "family@example.com",
                    allURL = listOf("https://www.netflix.com/login"), password = "netflix-password",
                ),
            ),
        )
        allPasskey = mutableListOf(gitHubPasskey)
        allMember = mutableListOf(
            Member("alice-id", "Alice", "alice@example.com", Member.ROLE_ADMIN, "1111 2222", isCurrentUser = true),
            Member("bob-id", "Bob", "bob@example.com", Member.ROLE_MEMBER, "3333 4444"),
            Member("carol-id", "Carol", "carol@example.com", Member.ROLE_MEMBER, "5555 6666"),
        )
        allVaultMemberByVaultID = mutableMapOf(
            "family" to mutableListOf(
                Member("bob-id", "Bob", "bob@example.com", Member.VAULT_ROLE_OWNER, "3333 4444"),
                Member("alice-id", "Alice", "alice@example.com", Member.VAULT_ROLE_EDITOR, "1111 2222", isCurrentUser = true),
            ),
        )
    }

    private val currentMember: Member? get() = allMember.firstOrNull { it.isCurrentUser }

    private fun requireUnlocked() {
        if (!status.isUnlocked) throw BackendError(if (status.isSignedIn) "Hisar is locked" else "Sign in first")
    }

    override fun createVault(name: String): VaultSummary {
        record("createVault")
        requireUnlocked()
        val trimmedName = name.trimmingWhitespaces()
        if (trimmedName.isEmpty()) throw BackendError("Enter a vault name")
        val vault = VaultSummary(id = "vault-${allVault.size + 1}", name = trimmedName, type = "shared", role = Member.VAULT_ROLE_OWNER, itemCount = 0)
        allVault += vault
        allItemByVaultID[vault.id] = mutableListOf()
        currentMember?.let { allVaultMemberByVaultID[vault.id] = mutableListOf(it.copy(role = Member.VAULT_ROLE_OWNER)) }
        return vault
    }

    override fun listAllPerson(): List<Member> {
        record("listAllPerson")
        requireUnlocked()
        val allSharedAccountID = allVaultMemberByVaultID.values.filter { members -> members.any { it.isCurrentUser } }
            .flatMap { members -> members.map { it.accountID } }.toSet()
        return allMember.filter { it.isCurrentUser || it.accountID in allSharedAccountID }
    }

    override fun invitePerson(email: String) {
        record("invitePerson")
        requireUnlocked()
        if ("@" !in email) throw BackendError("Enter the email address of the person")
        allInvitedEmail += email
    }

    override fun listAllShareInvitation(): ShareInvitations {
        record("listAllShareInvitation")
        requireUnlocked()
        return shareInvitations
    }

    override fun listAllShareAwaitingUnlock(): List<ShareAwaitingUnlock> {
        record("listAllShareAwaitingUnlock")
        if (isShareAwaitingUnlockFailing) throw BackendError("cannot reach the Hisar server")
        return if (status.state == VaultStatus.STATE_LOCKED) allShareAwaitingUnlock else emptyList()
    }

    override fun acceptShareInvitation(invitationID: String) {
        record("acceptShareInvitation")
        answerReceivedInvitation(invitationID, "accept")
    }

    override fun declineShareInvitation(invitationID: String) {
        record("declineShareInvitation")
        answerReceivedInvitation(invitationID, "decline")
    }

    private fun answerReceivedInvitation(invitationID: String, answer: String) {
        requireUnlocked()
        if (shareInvitations.allReceived.none { it.id == invitationID }) throw BackendError("This invitation no longer exists")
        shareInvitations = shareInvitations.copy(allReceived = shareInvitations.allReceived.filter { it.id != invitationID })
        allAnsweredInvitation += "$invitationID $answer"
    }

    override fun cancelShareInvitation(invitationID: String) {
        record("cancelShareInvitation")
        requireUnlocked()
        if (shareInvitations.allSent.none { it.id == invitationID }) throw BackendError("This invitation no longer exists")
        shareInvitations = shareInvitations.copy(allSent = shareInvitations.allSent.filter { it.id != invitationID })
    }

    override fun confirmShareRecipient(invitationID: String, reviewToken: String) {
        record("confirmShareRecipient")
        requireUnlocked()
        val invitation = shareInvitations.allSent.firstOrNull { it.id == invitationID }
            ?: throw BackendError("This invitation no longer exists")
        if (!invitation.requiresConfirmation || invitation.reviewToken != reviewToken || reviewToken.isEmpty()) {
            throw BackendError("Review this invitation again before confirming the recipient.")
        }
        findSharableVault(invitation.vaultID)
        shareInvitations = shareInvitations.copy(allSent = shareInvitations.allSent.filter { it.id != invitationID })
    }

    override fun listAllVaultMember(vaultID: String): List<Member> {
        record("listAllVaultMember")
        requireUnlocked()
        if (allVault.none { it.id == vaultID }) throw BackendError("Vault not found")
        return allVaultMemberByVaultID[vaultID].orEmpty()
    }

    override fun getVaultSharing(vaultID: String): VaultSharing {
        record("getVaultSharing")
        requireUnlocked()
        val vault = allVault.firstOrNull { it.id == vaultID } ?: throw BackendError("Vault not found")
        if (vault.type != "shared") throw BackendError("Personal vaults are private and cannot be shared; create a shared vault instead")
        val isOwner = vault.role == Member.VAULT_ROLE_OWNER
        return VaultSharing(
            vault = vault,
            isOwner = isOwner,
            allMember = allVaultMemberByVaultID[vaultID].orEmpty(),
            allInvitation = if (isOwner) shareInvitations.allSent.filter { it.vaultID == vaultID } else emptyList(),
            completionError = if (isOwner) shareInvitations.completionError else "",
        )
    }

    private fun findSharableVault(vaultID: String): VaultSummary {
        val vault = allVault.firstOrNull { it.id == vaultID } ?: throw BackendError("Vault not found")
        if (vault.type != "shared") throw BackendError("Personal vaults are private and cannot be shared; create a shared vault instead")
        if (vault.role != Member.VAULT_ROLE_OWNER) throw BackendError("Only owners of this vault can share it, change roles or remove members")
        return vault
    }

    override fun shareVault(vaultID: String, email: String, role: String) {
        record("shareVault")
        requireUnlocked()
        if (role !in Member.allVaultRole) throw BackendError("Choose owner, editor or reader")
        val vault = findSharableVault(vaultID)
        if ("@" !in email) throw BackendError("Enter the email address of the person")
        if (email == currentMember?.email) throw BackendError("That is your own email; you already have access")
        if (allVaultMemberByVaultID[vault.id].orEmpty().any { it.email == email }) throw BackendError("This person already has access to the vault")
        val allSent = shareInvitations.allSent.filterNot { it.vaultID == vault.id && it.email == email }
        val invitation = SentShareInvitation(
            id = "invitation-${allSent.size + 1}", vaultID = vault.id, vaultName = vault.name, email = email, role = role,
            state = "pending", expiresAt = "2027-01-22T08:00:00Z",
        )
        shareInvitations = shareInvitations.copy(allSent = allSent + invitation)
    }

    override fun changeVaultMemberRole(vaultID: String, accountID: String, role: String) {
        record("changeVaultMemberRole")
        requireUnlocked()
        if (role !in Member.allVaultRole) throw BackendError("Choose owner, editor or reader")
        val vault = findSharableVault(vaultID)
        val allVaultMember = allVaultMemberByVaultID[vault.id]
        val index = allVaultMember?.indexOfFirst { it.accountID == accountID } ?: -1
        if (allVaultMember == null || index < 0) {
            throw BackendError("This person no longer has access to the vault; share it with them again")
        }
        allVaultMember[index] = allVaultMember[index].copy(role = role)
    }

    override fun removeVaultMember(vaultID: String, accountID: String) {
        record("removeVaultMember")
        requireUnlocked()
        if (accountID == currentMember?.accountID) throw BackendError("You cannot remove yourself from a vault you own")
        val vault = findSharableVault(vaultID)
        allVaultMemberByVaultID[vault.id]?.removeAll { it.accountID == accountID }
    }

    override fun getStatus(): VaultStatus {
        record("getStatus")
        return status
    }

    var onCheckMinimumVersion: (() -> Unit)? = null

    override fun checkMinimumVersion(): VaultStatus {
        record("checkMinimumVersion")
        status = status.copy(updateRequired = serverUpdateRequirement)
        val answer = status
        onCheckMinimumVersion?.invoke()
        return answer
    }

    override fun signIn(server: String, email: String, secretKey: String, password: String): VaultStatus {
        record("signIn")
        if (password != PASSWORD || secretKey != SECRET_KEY) throw BackendError("Incorrect email, master password or Secret Key")
        seedSignedIn()
        status = status.copy(server = server, email = email)
        return status
    }

    override fun signUp(inviteLink: String, email: String, name: String, password: String): SignUpResult {
        record("signUp")
        if ("/invite/" !in inviteLink) throw BackendError("Invalid invite link")
        pendingSignUpResult?.let { return it }
        seedSignedIn()
        allItemByVaultID = mutableMapOf("personal" to mutableListOf(), "family" to mutableListOf())
        status = status.copy(email = email, name = name)
        return startNewAccount()
    }

    override fun signUpWithoutInvite(server: String, email: String, name: String, password: String): SignUpResult {
        record("signUpWithoutInvite")
        signUpWithoutInviteServer = server
        signUpWithoutInviteErrorMessage?.let { throw BackendError(it) }
        pendingSignUpResult?.let { return it }
        seedSignedIn()
        allItemByVaultID = mutableMapOf("personal" to mutableListOf(), "family" to mutableListOf())
        status = status.copy(server = server, email = email, name = name)
        return startNewAccount()
    }

    /** As the backend answers a sign-up that finished: its pending record waits for the kit, and the Secret Key is kept. */
    private fun startNewAccount(): SignUpResult {
        allPendingRecovery += PendingRecovery(
            operationId = NEW_ACCOUNT_OPERATION_ID, kind = "signup", origin = status.server, accountId = "new-account",
            email = status.email, name = status.name, phase = "delivery-pending", keychainProtected = true,
        )
        isNewAccountSecretKeyKept = true
        return SignUpResult(status, SECRET_KEY, NEW_ACCOUNT_OPERATION_ID, "delivery-pending")
    }

    override fun unlock(password: String): VaultStatus {
        record("unlock")
        if (password != PASSWORD) throw BackendError("Incorrect email, master password or Secret Key")
        status = status.copy(state = VaultStatus.STATE_UNLOCKED, biometricReady = status.biometricReady || status.biometricEnabled)
        return status
    }

    override fun unlockWithBiometric(): VaultStatus {
        record("unlockWithBiometric")
        if (!status.biometricReady) throw BackendError("Unlock with your master password first")
        if (!isBiometricSucceeding) throw BackendError("Face ID was cancelled.")
        status = status.copy(state = VaultStatus.STATE_UNLOCKED)
        return status
    }

    override fun enableBiometricUnlock(password: String): VaultStatus {
        record("enableBiometricUnlock")
        requireUnlocked()
        if (password != PASSWORD) throw BackendError("Incorrect email, master password or Secret Key")
        status = status.copy(biometricEnabled = true, biometricReady = true)
        return status
    }

    override fun enableNewAccountBiometricUnlock(): VaultStatus {
        record("enableNewAccountBiometricUnlock")
        requireUnlocked()
        if (!isBiometricSucceeding) throw BackendError("Face ID was cancelled.")
        status = status.copy(biometricEnabled = true, biometricReady = true)
        return status
    }

    override fun forgetNewAccountSecretKey() {
        record("forgetNewAccountSecretKey")
        isNewAccountSecretKeyKept = false
    }

    private fun findNewAccountRecovery(): Int {
        requireUnlocked()
        if (!isNewAccountSecretKeyKept) throw BackendError("the new account's Secret Key is no longer in memory; save the Emergency Kit from Settings")
        val index = allPendingRecovery.indexOfFirst { it.operationId == NEW_ACCOUNT_OPERATION_ID }
        if (index < 0) throw BackendError("this account's setup needs finishing from the account recovery screen")
        return index
    }

    override fun exportNewAccountEmergencyKit(directory: File): EmergencyKitExport {
        record("exportNewAccountEmergencyKit")
        val index = findNewAccountRecovery()
        val kit = writeEmergencyKit(directory)
        allPendingRecovery[index] = allPendingRecovery[index].copy(kitSaved = true)
        return EmergencyKitExport(SECRET_KEY, kit.path)
    }

    override fun acknowledgeNewAccountRecovery() {
        record("acknowledgeNewAccountRecovery")
        val index = findNewAccountRecovery()
        if (!allPendingRecovery[index].kitSaved) throw BackendError("finish setup and save the Emergency Kit before acknowledging delivery")
        allPendingRecovery.removeAt(index)
    }

    override fun disableBiometricUnlock(): VaultStatus {
        record("disableBiometricUnlock")
        status = status.copy(biometricEnabled = false, biometricReady = false)
        return status
    }

    override fun lock(): VaultStatus {
        record("lock")
        if (status.isSignedIn) status = status.copy(state = VaultStatus.STATE_LOCKED)
        isNewAccountSecretKeyKept = false
        return status
    }

    override fun signOut(): VaultStatus {
        record("signOut")
        // As the backend does, signing out keeps only the server, so the sign-in form opens on it. Offline it still signs
        // out on this device, then reports that the server was not told.
        status = VaultStatus.signedOut.copy(server = status.server)
        isNewAccountSecretKeyKept = false
        if (isOffline) throw BackendError(SIGN_OUT_OFFLINE_MESSAGE)
        return status
    }

    override fun reportActivity() {
        record("reportActivity")
    }

    override fun listAllVault(): List<VaultSummary> {
        record("listAllVault")
        requireUnlocked()
        return allVault.toList()
    }

    override fun listAllItem(vaultID: String): List<ItemDetail> {
        record("listAllItem")
        requireUnlocked()
        return allItemByVaultID[vaultID].orEmpty().map { item ->
            item.copy(
                isFavorite = LoginMatch.buildRecordIdentifier(vaultID, item.id) in allFavoriteRecordID,
                password = "",
                notes = "",
                allField = emptyList(),
                oneTimePassword = "",
            )
        }
    }

    override fun getItem(vaultID: String, itemID: String): ItemDetail {
        record("getItem")
        requireUnlocked()
        val item = allItemByVaultID[vaultID]?.firstOrNull { it.id == itemID } ?: throw BackendError("Item not found")
        return item.copy(isFavorite = LoginMatch.buildRecordIdentifier(vaultID, itemID) in allFavoriteRecordID)
    }

    override fun saveItem(vaultID: String, item: ItemDetail): ItemDetail {
        record("saveItem")
        requireUnlocked()
        if (item.title.trimmingWhitespaces().isEmpty()) throw BackendError("Enter a title")
        val oneTimePassword = item.oneTimePassword.trimmingWhitespacesAndNewlines()
        val isSecret = oneTimePassword.replace(" ", "").all { it.uppercaseChar() in "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567" }
        if (oneTimePassword.isNotEmpty() && !oneTimePassword.startsWith("otpauth://totp/") && !isSecret) {
            throw BackendError("The one-time code is not a valid secret or otpauth:// link")
        }
        var savedItem = item.copy(
            oneTimePassword = when {
                oneTimePassword.isEmpty() -> ""
                oneTimePassword.startsWith("otpauth://") -> oneTimePassword
                else -> "otpauth://totp/?secret=${oneTimePassword.uppercase().replace(" ", "")}"
            },
            hasOneTimePassword = oneTimePassword.isNotEmpty(),
        )
        if (savedItem.isCreditCard) {
            val card = savedItem.card ?: ItemCard()
            val description = describeCard(card.number, card.expiry)
            savedItem = savedItem.copy(
                card = card.copy(
                    brand = description.brand,
                    type = card.type.ifEmpty { description.brand },
                    last4 = description.last4,
                    numberWarning = description.numberWarning,
                ),
            )
        }
        if (savedItem.id.isEmpty()) {
            savedItem = savedItem.copy(id = "new-$nextItemNumber")
            nextItemNumber += 1
        }
        savedItem = savedItem.copy(revision = UUID.randomUUID().toString())
        val allItem = allItemByVaultID.getOrPut(vaultID) { mutableListOf() }
        val index = allItem.indexOfFirst { it.id == savedItem.id }
        if (index >= 0) allItem[index] = savedItem else allItem += savedItem
        return savedItem
    }

    /** A stand-in for core/item's checks: Visa by the first digit, and a number of 16 digits ending in 1 counts as valid. */
    override fun describeCard(number: String, expiry: String): CardDescription {
        record("describeCard")
        val digits = number.filter { it.isNumber() }
        val isValid = digits.length == 16 && digits.endsWith("1")
        return CardDescription(
            brand = if (digits.startsWith("4")) "Visa" else "",
            last4 = digits.takeLast(4),
            numberWarning = if (number.isEmpty() || isValid) "" else "This doesn't look like a valid card number",
            expiryWarning = "",
        )
    }

    override fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail {
        record("convertToSecureNote")
        val item = getItem(vaultID, itemID)
        if (!item.isLogin || item.username.isNotEmpty() || item.password.isNotEmpty() || item.allURL.isNotEmpty() ||
            (item.notes.isEmpty() && item.allField.isEmpty())
        ) {
            throw BackendError("only a login with nothing but notes and custom fields can become a secure note")
        }
        val converted = item.copy(category = ItemDetail.CATEGORY_SECURE_NOTE, canConvertToSecureNote = false, revision = UUID.randomUUID().toString())
        replaceItem(vaultID, converted)
        return converted
    }

    override fun listAllFavoriteItem(): List<FavoriteItem> {
        record("listAllFavoriteItem")
        requireUnlocked()
        return allVault.flatMap { vault ->
            runCatching { listAllItem(vault.id) }.getOrDefault(emptyList()).filter { it.isFavorite }
                .map { FavoriteItem(vault.id, vault.name, vault.type, it) }
        }
    }

    override fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean) {
        record("setItemFavorite")
        requireUnlocked()
        favoriteErrorMessage?.let { throw BackendError(it) }
        val recordID = LoginMatch.buildRecordIdentifier(vaultID, itemID)
        if (isFavorite) allFavoriteRecordID += recordID else allFavoriteRecordID -= recordID
    }

    override fun deleteItem(vaultID: String, itemID: String) {
        record("deleteItem")
        requireUnlocked()
        allItemByVaultID[vaultID]?.removeAll { it.id == itemID }
        allPasskey.removeAll { it.vaultID == vaultID && it.itemID == itemID }
    }

    override fun deleteUnreadableItem(vaultID: String, itemID: String, revision: String) {
        requireUnlocked()
        if (allItemByVaultID[vaultID]?.any { it.id == itemID && it.revision == revision && it.readIssue?.canDelete == true } != true) throw BackendError("The unreadable record changed")
        allItemByVaultID[vaultID]?.removeAll { it.id == itemID }
        allVault = allVault.map { if (it.id == vaultID) it.copy(readIssues = it.readIssues.filter { issue -> issue.itemId != itemID }) else it }.toMutableList()
    }

    override fun deleteUnreadableVault(vaultID: String, revision: String) {
        requireUnlocked()
        if (allVault.none { it.id == vaultID && it.readIssues.any { issue -> issue.canDelete && issue.itemId.isEmpty() && issue.revision == revision } }) throw BackendError("The unreadable vault changed")
        allVault.removeAll { it.id == vaultID }
        allItemByVaultID.remove(vaultID)
    }

    override fun repairVaultName(vaultID: String, revision: String, name: String): VaultSummary {
        requireUnlocked()
        val index = allVault.indexOfFirst { it.id == vaultID && it.readIssues.any { issue -> issue.canRename && issue.revision == revision } }
        if (index < 0) throw BackendError("The unreadable vault changed")
        allVault[index] = allVault[index].copy(name = name.trim(), readIssues = allVault[index].readIssues.filter { it.itemId.isNotEmpty() })
        return allVault[index]
    }

    override fun getOneTimeCode(vaultID: String, itemID: String): OneTimeCode {
        record("getOneTimeCode")
        if (!getItem(vaultID, itemID).hasOneTimePassword) throw BackendError("One-time password not found")
        return oneTimeCode
    }

    override fun getOneTimeCodeForService(vaultID: String, itemID: String, serviceIdentifier: String): OneTimeCode {
        record("getOneTimeCodeForService")
        if (listAllLoginForService(serviceIdentifier).none { it.vaultID == vaultID && it.itemID == itemID }) {
            throw BackendError("This login is not saved for this site")
        }
        return getOneTimeCode(vaultID, itemID)
    }

    override fun copyField(vaultID: String, itemID: String, fieldName: String) {
        record("copyField")
        val item = getItem(vaultID, itemID)
        allCopiedField += "${item.id}.$fieldName"
    }

    override fun generatePassword(length: Int, isDigitIncluded: Boolean, isSymbolIncluded: Boolean): String {
        record("generatePassword")
        return (if (isSymbolIncluded) "!" else if (isDigitIncluded) "1" else "a").repeat(length)
    }

    override fun ratePassword(password: String): PasswordStrength {
        record("ratePassword")
        return rateMasterPassword(password).strength
    }

    override fun checkMasterPassword(password: String): MasterPasswordFeedback {
        record("checkMasterPassword")
        return rateMasterPassword(password)
    }

    /** core/item's answers for password1234 and the empty password; any other password rates Good. */
    private fun rateMasterPassword(password: String): MasterPasswordFeedback {
        val advice = "A few random words you can remember make a strong master password."
        return when (password) {
            "" -> MasterPasswordFeedback(message = advice)
            "password1234" ->
                MasterPasswordFeedback("weak", "Weak", 8, "This password is too easy to guess. Add a few more words or characters.", isBlocking = true)
            else -> MasterPasswordFeedback("good", "Good", 60, advice)
        }
    }

    override fun listAllLoginForService(serviceIdentifier: String): List<LoginMatch> {
        record("listAllLoginForService")
        requireUnlocked()
        val host = VaultLogic.extractHost(serviceIdentifier) ?: return emptyList()
        return allVault.flatMap { vault ->
            allItemByVaultID[vault.id].orEmpty().filter { item ->
                item.allURL.any { url -> VaultLogic.extractHost(url)?.let { host.endsWith(it) || it.endsWith(host) } ?: false }
            }.map { LoginMatch(vault.id, it.id, it.title, it.username, vault.name, it.hasOneTimePassword, isPersonal = vault.type == "personal") }
        }
    }

    override fun getLoginForService(vaultID: String, itemID: String, serviceIdentifier: String): LoginCredential {
        record("getLoginForService")
        if (listAllLoginForService(serviceIdentifier).none { it.vaultID == vaultID && it.itemID == itemID }) {
            throw BackendError("This login is not saved for this site")
        }
        val item = getItem(vaultID, itemID)
        return LoginCredential(item.username, item.password, item.hasOneTimePassword)
    }

    override fun listAllLoginForAndroidApp(packageName: String, certificateSHA256: String): List<LoginMatch> {
        record("listAllLoginForAndroidApp")
        requireUnlocked()
        val certificate = certificateSHA256.replace(":", "").lowercase()
        return allVault.flatMap { vault ->
            allItemByVaultID[vault.id].orEmpty().filter { item ->
                item.allAndroidApp.any { it.packageName == packageName && it.certificateSHA256.replace(":", "").lowercase() == certificate }
            }.map { LoginMatch(vault.id, it.id, it.title, it.username, vault.name, it.hasOneTimePassword, isPersonal = vault.type == "personal") }
        }
    }

    override fun getLoginForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): LoginCredential {
        record("getLoginForAndroidApp")
        if (listAllLoginForAndroidApp(packageName, certificateSHA256).none { it.vaultID == vaultID && it.itemID == itemID }) {
            throw BackendError("This login is not linked to this app")
        }
        val item = getItem(vaultID, itemID)
        return LoginCredential(item.username, item.password, item.hasOneTimePassword)
    }

    override fun getOneTimeCodeForAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): OneTimeCode {
        record("getOneTimeCodeForAndroidApp")
        if (listAllLoginForAndroidApp(packageName, certificateSHA256).none { it.vaultID == vaultID && it.itemID == itemID }) {
            throw BackendError("This login is not linked to this app")
        }
        return getOneTimeCode(vaultID, itemID)
    }

    /** Every login website's host once, sorted; the registrable domain is its last two labels. */
    override fun listAllLoginWebsite(): List<LoginWebsite> {
        record("listAllLoginWebsite")
        requireUnlocked()
        return allItemByVaultID.values.flatten().filter { it.offersLoginFill }.flatMap { it.allURL }
            .mapNotNull(VaultLogic::extractHost).distinct().sorted()
            .map { LoginWebsite(it, it.split(".").takeLast(2).joinToString(".")) }
    }

    override fun listAllPasskey(): List<StoredPasskey> {
        record("listAllPasskey")
        requireUnlocked()
        onListAllPasskey?.invoke()
        return allPasskey.toList()
    }

    override fun listAllPasskeyForRelyingParty(relyingPartyID: String, allAllowedCredentialID: List<ByteString>): List<StoredPasskey> {
        record("listAllPasskeyForRelyingParty")
        requireUnlocked()
        val allAllowedID = allAllowedCredentialID.map(Base64URL::encode).toSet()
        return allPasskey.filter { it.relyingPartyID == relyingPartyID && (allAllowedID.isEmpty() || it.credentialID in allAllowedID) }
    }

    override fun preparePasskeyRegistration(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): PasskeyRegistrationOptions {
        record("preparePasskeyRegistration")
        val allLogin = listAllLoginForService("https://$relyingPartyID").filter { login ->
            allVault.firstOrNull { it.id == login.vaultID }?.role != "reader"
        }
        return PasskeyRegistrationOptions(checkIsExcluded(relyingPartyID, allExcludedCredentialID), allLogin)
    }

    private fun checkIsExcluded(relyingPartyID: String, allExcludedCredentialID: List<ByteString>): Boolean {
        val allExcludedID = allExcludedCredentialID.map(Base64URL::encode).toSet()
        return allPasskey.any { it.relyingPartyID == relyingPartyID && it.credentialID in allExcludedID }
    }

    override fun createPasskey(request: PasskeyRegistrationRequest, vaultID: String, itemID: String): PasskeyRegistrationResult {
        record("createPasskey")
        requireUnlocked()
        if (request.userVerification != "discouraged" && !verifyPasskey("Create a passkey for ${request.relyingPartyID}")) throw BackendError("Passkey verification was cancelled.", BackendError.PASSKEY_VERIFICATION_FAILED)
        if (checkIsExcluded(request.relyingPartyID, request.allExcludedCredentialID)) {
            throw BackendError("Hisar already has a passkey for this account")
        }
        if (request.clientDataHash.size != 32) throw BackendError("the request is not valid")
        val targetVaultID: String
        var item: ItemDetail
        if (vaultID.isEmpty()) {
            targetVaultID = "personal"
            item = saveItem(
                targetVaultID,
                ItemDetail(title = request.relyingPartyID, username = request.userName, allURL = listOf("https://${request.relyingPartyID}")),
            )
        } else {
            targetVaultID = vaultID
            val isListed = preparePasskeyRegistration(request.relyingPartyID, emptyList()).allLogin.any { it.vaultID == vaultID && it.itemID == itemID }
            if (!isListed) throw BackendError("this login is not saved for this site")
            item = getItem(vaultID, itemID)
        }
        val userHandle = Base64URL.encode(request.userHandle)
        val credentialID = Base64URL.encode(ByteString((allPasskey.size + 2).toByte(), 7, 7))
        val replacedID = allPasskey.filter {
            it.vaultID == targetVaultID && it.itemID == item.id && it.relyingPartyID == request.relyingPartyID && it.userHandle == userHandle
        }.map { it.credentialID }.toSet()
        allPasskey.removeAll { it.credentialID in replacedID }
        allPasskey += StoredPasskey(credentialID, request.relyingPartyID, request.userName, "", userHandle, targetVaultID, item.id, item.title)
        item = item.copy(
            allPasskey = item.allPasskey.filter { it.credentialID !in replacedID } +
                ItemPasskey(credentialID, request.relyingPartyID, request.userName),
        )
        replaceItem(targetVaultID, item)
        return PasskeyRegistrationResult(
            credentialID = credentialID,
            attestationObject = Base64URL.encode("attestation for ${request.relyingPartyID}".encodeToByteString()),
            authenticatorData = Base64URL.encode("authenticator data".encodeToByteString()),
            vaultID = targetVaultID,
            itemID = item.id,
        )
    }

    private fun replaceItem(vaultID: String, item: ItemDetail) {
        val allItem = allItemByVaultID[vaultID] ?: return
        val index = allItem.indexOfFirst { it.id == item.id }
        if (index >= 0) allItem[index] = item
    }

    override fun assertPasskey(relyingPartyID: String, credentialID: ByteString, clientDataHash: ByteString, userVerification: String): PasskeyAssertionResult {
        record("assertPasskey")
        requireUnlocked()
        if (userVerification != "discouraged" && !verifyPasskey("Use your passkey for $relyingPartyID")) throw BackendError("Passkey verification was cancelled.", BackendError.PASSKEY_VERIFICATION_FAILED)
        if (clientDataHash.size != 32) throw BackendError("the request is not valid")
        val passkey = allPasskey.firstOrNull { it.relyingPartyID == relyingPartyID && it.credentialID == Base64URL.encode(credentialID) }
            ?: throw BackendError("passkey not found")
        return PasskeyAssertionResult(
            credentialID = passkey.credentialID,
            authenticatorData = Base64URL.encode("authenticator data for $relyingPartyID".encodeToByteString()),
            signature = Base64URL.encode(ByteString("signature over ".encodeToByteString().toByteArray() + clientDataHash.toByteArray())),
            userHandle = passkey.userHandle,
        )
    }

    // The host or a parent domain of it, as core/passkey accepts them; the public suffix check is Go's and is left out.
    override fun resolvePasskeyRelyingPartyID(origin: String, relyingPartyID: String): String {
        record("resolvePasskeyRelyingPartyID")
        val host = Regex("^https://([^/:]+)/?$").find(origin)?.groupValues?.get(1) ?: throw BackendError("the origin is not https, localhost or an Android app")
        if (relyingPartyID.isEmpty()) return host
        if (relyingPartyID != host && !host.endsWith(".$relyingPartyID")) throw BackendError("the relying party ID does not belong to the origin")
        return relyingPartyID
    }

    // A lower-case domain name with a dot and no IP address, as core/passkey accepts it; the public suffix check is Go's.
    override fun validatePasskeyRelyingPartyID(relyingPartyID: String): String {
        record("validatePasskeyRelyingPartyID")
        val normalized = relyingPartyID.trim().lowercase()
        val label = "[a-z0-9]([a-z0-9-]*[a-z0-9])?"
        if (!Regex("^($label\\.)+$label$").matches(normalized) || Regex("^[0-9.]+$").matches(normalized)) {
            throw BackendError("the relying party ID does not belong to the origin")
        }
        return normalized
    }

    override fun buildAndroidAppOrigin(certificateSHA256: ByteString): String =
        "android:apk-key-hash:" + Base64URL.encode(certificateSHA256)

    override fun buildPasskeyClientData(clientDataType: String, challenge: String, origin: String, androidPackageName: String): PasskeyClientData {
        record("buildPasskeyClientData")
        val json = """{"type":"$clientDataType","challenge":"$challenge","origin":"$origin","crossOrigin":false,"androidPackageName":"$androidPackageName"}"""
            .encodeToByteString()
        return PasskeyClientData(json, ByteString(java.security.MessageDigest.getInstance("SHA-256").digest(json.toByteArray())))
    }

    override fun deletePasskey(vaultID: String, itemID: String, credentialID: String): ItemDetail {
        record("deletePasskey")
        val item = getItem(vaultID, itemID)
        if (item.allPasskey.none { it.credentialID == credentialID }) throw BackendError("passkey not found")
        val updated = item.copy(allPasskey = item.allPasskey.filter { it.credentialID != credentialID })
        allPasskey.removeAll { it.credentialID == credentialID }
        replaceItem(vaultID, updated)
        return updated
    }

    // Phone approvals: the Mac's requests are seeded and answers recorded. Listing is not recorded, as it runs in the background.
    override fun listAllPendingApproval(): List<PhoneApproval> {
        requireUnlocked()
        val allApproval = withApprovalLock { allPendingApproval.toList() }
        onListAllPendingApproval?.invoke()
        return allApproval
    }

    override fun approveApproval(requestID: String, isRemembered: Boolean) {
        record("approveApproval")
        requireUnlocked()
        if (!isBiometricAvailable) throw BackendError("Face ID is not available; approve with your master password")
        if (!isBiometricSucceeding) throw BackendError("Face ID was cancelled.")
        answerApproval(requestID, if (isRemembered) "approve remembered" else "approve")
    }

    override fun approveApprovalWithPassword(requestID: String, password: String, isRemembered: Boolean) {
        record("approveApprovalWithPassword")
        requireUnlocked()
        if (password != PASSWORD) throw BackendError("Incorrect master password.")
        answerApproval(requestID, if (isRemembered) "approve remembered" else "approve")
    }

    override fun reconnectEvents() {
        record("reconnectEvents")
    }

    override fun denyApproval(requestID: String) {
        record("denyApproval")
        requireUnlocked()
        answerApproval(requestID, "deny")
    }

    private fun answerApproval(requestID: String, decision: String) = withApprovalLock {
        if (allPendingApproval.none { it.requestID == requestID }) throw BackendError("This request is no longer waiting for an answer")
        allPendingApproval.removeAll { it.requestID == requestID }
        allApprovalAnswer += "$decision $requestID"
    }

    override fun setAutoLock(seconds: Int): VaultStatus {
        record("setAutoLock")
        if (seconds !in VaultStatus.allAutoLockSeconds) throw BackendError("choose 30 seconds, 1 minute, 5 minutes, 10 minutes or 1 hour")
        status = status.copy(autoLockSeconds = seconds)
        return status
    }

    override fun listAllDevice(): List<Device> {
        record("listAllDevice")
        requireUnlocked()
        listAllDeviceErrorMessage?.let { throw BackendError(it) }
        return allDevice.toList()
    }

    override fun revokeDevice(deviceID: String) {
        record("revokeDevice")
        requireUnlocked()
        val device = allDevice.firstOrNull { it.id == deviceID } ?: throw BackendError("this device is already signed out")
        if (device.isCurrent) throw BackendError("sign out this device instead")
        allDevice.removeAll { it.id == deviceID }
    }

    override fun getVaultActions(vaultID: String): VaultActions {
        record("getVaultActions")
        requireUnlocked()
        val vault = allVault.firstOrNull { it.id == vaultID } ?: throw BackendError("vault not found")
        val actions = VaultActions(
            vault = vault,
            canRename = vault.role == Member.VAULT_ROLE_OWNER,
            allMemberLosingAccess = allVaultMemberByVaultID[vaultID].orEmpty().filter { !it.isCurrentUser },
        )
        return when {
            vault.type == "personal" -> actions.copy(
                deleteBlockedReason = "Your Personal vault can't be deleted.",
                leaveBlockedReason = "You can't leave your Personal vault.",
            )
            vault.role != Member.VAULT_ROLE_OWNER -> actions.copy(
                deleteBlockedReason = "Only an owner of this vault can delete it. You can leave it instead.",
                canLeave = true,
            )
            else -> actions.copy(
                canDelete = true,
                leaveBlockedReason = "You are its only owner.",
                allTargetVault = allVault.filter { it.id != vaultID && it.role != Member.VAULT_ROLE_READER },
            )
        }
    }

    override fun renameVault(vaultID: String, name: String): VaultSummary {
        record("renameVault")
        requireUnlocked()
        val trimmedName = name.trimmingWhitespaces()
        if (trimmedName.isEmpty()) throw BackendError("Enter a vault name")
        val index = allVault.indexOfFirst { it.id == vaultID }
        if (index < 0 || allVault[index].role != Member.VAULT_ROLE_OWNER) throw BackendError("Only an owner of the vault can do that")
        allVault[index] = allVault[index].copy(name = trimmedName)
        return allVault[index]
    }

    override fun deleteVault(vaultID: String, request: VaultDeletionRequest): VaultDeletionResult {
        record("deleteVault")
        requireUnlocked()
        allDeletionRequest += request
        val vault = allVault.firstOrNull { it.id == vaultID } ?: throw BackendError("vault not found")
        val allItem = allItemByVaultID[vaultID].orEmpty()
        var result = VaultDeletionResult()
        if (request.moveToVaultID.isNotEmpty()) {
            val target = allVault.firstOrNull { it.id == request.moveToVaultID } ?: throw BackendError("vault not found")
            allItemByVaultID.getOrPut(target.id) { mutableListOf() } += allItem
            result = VaultDeletionResult(allItem.size, target.name)
        } else if (request.isDeletingItems) {
            if (request.confirmedName.trimmingWhitespaces() != vault.name) {
                throw BackendError("Type the vault's name to delete it together with its items")
            }
        } else if (allItem.isNotEmpty()) {
            throw BackendError("This vault has items")
        }
        allVault.removeAll { it.id == vaultID }
        allItemByVaultID.remove(vaultID)
        return result
    }

    override fun leaveVault(vaultID: String) {
        record("leaveVault")
        requireUnlocked()
        allVault.removeAll { it.id == vaultID }
        allItemByVaultID.remove(vaultID)
    }

    override fun listAllRemovedVault(): List<RemovedVaultNotice> {
        record("listAllRemovedVault")
        requireUnlocked()
        val allNotice = allPendingRemovedVault
        allPendingRemovedVault = emptyList()
        return allNotice
    }

    override fun getExportOptions(): ExportOptions {
        record("getExportOptions")
        return exportOptions
    }

    override fun exportAllData(format: ExportFormat, passphrase: String, password: String, directory: File): ExportResult {
        record("exportAllData")
        requireUnlocked()
        if (password.isEmpty()) {
            if (!isBiometricSucceeding) throw BackendError("Face ID was cancelled.")
        } else if (password != PASSWORD) {
            throw BackendError("Incorrect email, master password or Secret Key")
        }
        directory.mkdirs()
        val file = File(directory, if (format == ExportFormat.PLAIN) "Hisar export (not encrypted).json" else "Hisar export.hisar")
        file.writeText("{}")
        return ExportResult(file.path, allVault.size, allItemByVaultID.values.sumOf { it.size })
    }

    private fun checkImportVault(vaultID: String): VaultSummary {
        val vault = allVault.firstOrNull { it.id == vaultID } ?: throw BackendError("Vault not found")
        if (vault.type != VaultSummary.TYPE_PERSONAL && vault.role == Member.VAULT_ROLE_READER) throw BackendError("You can only view this vault")
        return vault
    }

    /** The pending import as core/app previews it: vaultID is the vault every group goes to, or "" when they go to different vaults. */
    private fun buildImportPreview(): ImportPreview {
        val allGroup = allPendingImportGroup ?: throw BackendError("There is no import to continue")
        val commonVault = allGroup.map { it.vaultID }.toSet().singleOrNull()?.let(::checkImportVault)
        val allDuplicate = mutableListOf<ImportDuplicate>()
        val allSourceGroup = allGroup.map { group ->
            val vault = checkImportVault(group.vaultID)
            val allExistingTitle = allItemByVaultID[vault.id].orEmpty().map { it.title }.toSet()
            allDuplicate += group.allTitle.filter { it in allExistingTitle }.map { ImportDuplicate(title = it) }
            ImportSourceGroup(group.id, group.name, true, group.allTitle.size, vault.id, vault.name, allVaultMemberByVaultID[vault.id].orEmpty())
        }
        val itemCount = allGroup.sumOf { it.allTitle.size }
        return ImportPreview(
            fileName = "Other Manager", vaultID = commonVault?.id.orEmpty(), vaultName = commonVault?.name ?: "the mapped vaults", itemCount = itemCount,
            loginCount = itemCount, allDuplicate = allDuplicate, groups = allSourceGroup,
        )
    }

    /** Reads what the other app exported as the titles of the logins to import, or as one list of titles per collection. */
    override fun prepareCredentialExchangeImport(vaultID: String, credentialData: ByteArray): ImportPreview {
        record("prepareCredentialExchangeImport")
        requireUnlocked()
        val text = credentialData.decodeToString()
        allPendingImportGroup = runCatching { backendJson.decodeFromString(ListSerializer(String.serializer()), text) }.map { allTitle ->
            listOf(FakeImportGroup("private", "Private or unspecified source", allTitle, vaultID))
        }.getOrElse {
            backendJson.decodeFromString(ListSerializer(ListSerializer(String.serializer())), text).mapIndexed { index, allTitle ->
                FakeImportGroup("group-$index", "Collection ${index + 1}", allTitle, vaultID)
            }
        }
        allImportedCredentialData += credentialData.copyOf()
        return buildImportPreview()
    }

    /** Reads a Hisar export: an encrypted one waits for its passphrase, a plain one previews at once. */
    override fun prepareHisarExportImport(vaultID: String, fileName: String, content: ByteArray): ImportPreview {
        record("prepareHisarExportImport")
        requireUnlocked()
        if (content.isEmpty()) throw BackendError("this is not a Hisar export")
        if (isHisarExportEncrypted) {
            waitingHisarExport = vaultID to fileName
            return ImportPreview(
                fileName = fileName, format = "hisar-encrypted-export", formatName = "Hisar encrypted export",
                isPassphraseRequired = true, isSourceKept = true,
            )
        }
        return openHisarExport(vaultID, fileName)
    }

    /** Opens the waiting encrypted Hisar export; only [HISAR_EXPORT_PASSPHRASE] opens it, and a wrong one keeps it waiting. */
    override fun openImportFile(passphrase: String): ImportPreview {
        record("openImportFile")
        requireUnlocked()
        val (vaultID, fileName) = waitingHisarExport ?: throw BackendError("this Hisar export is encrypted; enter its export passphrase")
        if (passphrase != HISAR_EXPORT_PASSPHRASE) throw BackendError("that passphrase doesn't open this export")
        waitingHisarExport = null
        return openHisarExport(vaultID, fileName)
    }

    private fun openHisarExport(vaultID: String, fileName: String): ImportPreview {
        allPendingImportGroup = listOf(FakeImportGroup("personal", "Personal", allHisarExportTitle, vaultID))
        return buildImportPreview().copy(
            fileName = fileName, format = "hisar-export", formatName = "Hisar export",
            notice = hisarExportNotice, exportedFrom = hisarExportFrom, exportedAt = hisarExportAt,
        )
    }

    override fun previewImport(vaultID: String): ImportPreview {
        record("previewImport")
        requireUnlocked()
        val group = allPendingImportGroup?.singleOrNull() ?: throw BackendError("Choose a destination for each source group")
        checkImportVault(vaultID)
        group.vaultID = vaultID
        return buildImportPreview()
    }

    override fun mapImportGroup(groupID: String, vaultID: String): ImportPreview {
        record("mapImportGroup")
        requireUnlocked()
        val group = allPendingImportGroup?.firstOrNull { it.id == groupID } ?: throw BackendError("Source group not found; choose the import file again")
        checkImportVault(vaultID)
        group.vaultID = vaultID
        return buildImportPreview()
    }

    override fun startImport(vaultID: String, isSkippingDuplicate: Boolean): ImportProgress {
        record("startImport")
        requireUnlocked()
        if (vaultID != buildImportPreview().vaultID) throw BackendError("Review the import destinations again")
        allQueuedImport = allPendingImportGroup.orEmpty().flatMap { group ->
            val allExistingTitle = allItemByVaultID[group.vaultID].orEmpty().map { it.title }.toSet()
            group.allTitle.filter { !isSkippingDuplicate || it !in allExistingTitle }.map { it to group.vaultID }
        }
        pendingImportVaultID = vaultID
        importSavedCount = 0
        return buildImportProgress()
    }

    override fun continueImport(): ImportProgress {
        record("continueImport")
        requireUnlocked()
        if (allPendingImportGroup == null) throw BackendError("There is no import to continue")
        continueImportErrorMessage?.let { message ->
            continueImportErrorMessage = null
            throw BackendError(message)
        }
        for ((title, vaultID) in allQueuedImport.take(importBatchSize)) {
            allItemByVaultID.getOrPut(vaultID) { mutableListOf() } +=
                ItemDetail(id = "imported-$nextItemNumber", revision = "r1", category = "login", title = title)
            nextItemNumber += 1
            importSavedCount += 1
        }
        allQueuedImport = allQueuedImport.drop(importBatchSize)
        return buildImportProgress()
    }

    private fun buildImportProgress(): ImportProgress = ImportProgress(
        fileName = "Other Manager", vaultID = pendingImportVaultID, vaultName = allVault.firstOrNull { it.id == pendingImportVaultID }?.name ?: "the mapped vaults",
        savedCount = importSavedCount, totalCount = importSavedCount + allQueuedImport.size, isDone = allQueuedImport.isEmpty(),
    )

    override fun cancelImport() {
        record("cancelImport")
        allPendingImportGroup = null
        allQueuedImport = emptyList()
        waitingHisarExport = null
    }

    override fun exportEmergencyKit(password: String, directory: File): EmergencyKitExport {
        record("exportEmergencyKit")
        requireUnlocked()
        if (password != PASSWORD) throw BackendError("Incorrect master password.")
        return EmergencyKitExport(SECRET_KEY, writeEmergencyKit(directory).path)
    }

    private fun writeEmergencyKit(directory: File): File {
        directory.mkdirs()
        return File(directory, "Hisar Emergency Kit.pdf").apply { writeText("%PDF-1.7") }
    }

    override fun changeMasterPassword(currentPassword: String, newPassword: String): CredentialChangeResult {
        record("changeMasterPassword")
        pendingCredentialChangeResult?.let { return it }
        requireUnlocked()
        if (currentPassword != this.currentPassword) throw BackendError("The master password is not correct")
        if (newPassword.length < 10) throw BackendError("Use a master password with at least 10 characters")
        this.currentPassword = newPassword
        return CredentialChangeResult(status, signedOutDeviceCount = 1)
    }

    override fun replaceSecretKey(password: String, directory: File): CredentialChangeResult {
        record("replaceSecretKey")
        pendingCredentialChangeResult?.let { return it }
        requireUnlocked()
        if (password != currentPassword) throw BackendError("The master password is not correct")
        currentSecretKey = "H1-ABC123-ZZZZZ-YYYYY-XXXXX-WWWWW-VVVVVV"
        val emergencyKit = writeEmergencyKit(directory)
        return CredentialChangeResult(status, secretKey = currentSecretKey, signedOutDeviceCount = 2, emergencyKitPath = emergencyKit.path)
    }

    override fun previewAccountDeletion(): AccountDeletionPreview {
        record("previewAccountDeletion")
        requireUnlocked()
        return deletionPreview
    }

    override fun deleteAccountWithPlan(password: String, email: String, planVersion: String): VaultStatus {
        record("deleteAccountWithPlan")
        requireUnlocked()
        if (planVersion.isEmpty() || planVersion != deletionPreview.planVersion) throw BackendError("The deletion plan changed. Review it again.")
        if (email.trimmingWhitespaces().lowercase() != status.email.lowercase()) throw BackendError("Type the email of this account to confirm")
        if (password != currentPassword) throw BackendError("The master password is not correct")
        isAccountDeleted = true
        status = VaultStatus.signedOut
        if (isCacheRemovalFailing) throw BackendError(CACHE_REMOVAL_MESSAGE)
        return status
    }

    override fun listPendingRecovery(): List<PendingRecovery> {
        record("listPendingRecovery")
        return allPendingRecovery.toList()
    }

    override fun resumePendingRecovery(operationID: String, password: String): SignUpResult {
        record("resumePendingRecovery")
        val index = allPendingRecovery.indexOfFirst { it.operationId == operationID }
        if (password != pendingRecoveryPassword || index < 0) throw BackendError("The recovery password is not correct.")
        if (allPendingRecovery[index].phase == "awaiting-kit") throw BackendError("Save and confirm the Emergency Kit before continuing.")
        if (!allPendingRecovery[index].keychainProtected && !isPendingRecoveryFactorAvailable) {
            throw BackendError("Enter the Secret Key from your saved Emergency Kit.")
        }
        allPendingRecovery[index] = allPendingRecovery[index].copy(phase = "delivery-pending")
        return SignUpResult(status, pendingRecoverySecretKey, operationID, "delivery-pending", allPendingRecovery[index].problems)
    }

    override fun resumePendingRecoveryWithSecretKey(operationID: String, password: String, secretKey: String, priorSecretKey: String): SignUpResult {
        record("resumePendingRecoveryWithSecretKey")
        if (secretKey != pendingRecoverySecretKey || (priorSecretKey.isNotEmpty() && priorSecretKey != currentSecretKey)) {
            throw BackendError("The Secret Key does not match this recovery.")
        }
        isPendingRecoveryFactorAvailable = true
        return resumePendingRecovery(operationID, password)
    }

    override fun discardPreparedRecovery(operationID: String, password: String) {
        record("discardPreparedRecovery")
        val recovery = allPendingRecovery.firstOrNull { it.operationId == operationID }
        if (password != pendingRecoveryPassword || recovery == null || (recovery.phase != "awaiting-kit" && recovery.phase != "prepared")) {
            throw BackendError("Only an uncommitted setup can be discarded.")
        }
        allPendingRecovery.removeAll { it.operationId == operationID }
    }

    /** Runs while a pending recovery's Emergency Kit is prepared: a test can lock or sign out meanwhile. */
    var onExportPendingRecoveryEmergencyKit: (() -> Unit)? = null

    override fun exportPendingRecoveryEmergencyKit(operationID: String, password: String, directory: File): PendingRecoveryExport {
        record("exportPendingRecoveryEmergencyKit")
        onExportPendingRecoveryEmergencyKit?.invoke()
        val index = allPendingRecovery.indexOfFirst { it.operationId == operationID }
        if (password != pendingRecoveryPassword || index < 0 || allPendingRecovery[index].phase == "outcome-unknown") {
            throw BackendError("Resume this recovery before preparing the Emergency Kit.")
        }
        if (!allPendingRecovery[index].keychainProtected && !isPendingRecoveryFactorAvailable) {
            throw BackendError("Enter the Secret Key from your saved Emergency Kit.")
        }
        val kit = writeEmergencyKit(directory)
        allPendingRecovery[index] = allPendingRecovery[index].copy(kitSaved = true)
        return PendingRecoveryExport(pendingRecoverySecretKey, kit.path)
    }

    override fun acknowledgePendingRecovery(operationID: String, password: String) {
        record("acknowledgePendingRecovery")
        val recovery = allPendingRecovery.firstOrNull { it.operationId == operationID }
        if (password != pendingRecoveryPassword || recovery == null ||
            (recovery.phase != "delivery-pending" && recovery.phase != "awaiting-kit") || !recovery.kitSaved
        ) {
            throw BackendError("Save the new Emergency Kit before finishing recovery.")
        }
        if (recovery.phase == "awaiting-kit" || recovery.manualUnlock) {
            val index = allPendingRecovery.indexOfFirst { it.operationId == operationID }
            allPendingRecovery[index] = recovery.copy(phase = if (recovery.phase == "awaiting-kit") "prepared" else "manual-unlock")
        } else {
            allPendingRecovery.removeAll { it.operationId == operationID }
        }
    }

    override fun dismissSignedOutReason(): VaultStatus {
        record("dismissSignedOutReason")
        status = status.copy(signedOutReason = "")
        return status
    }

    override fun copySecretReference(vaultID: String, itemID: String, fieldName: String): String {
        record("copySecretReference")
        val item = getItem(vaultID, itemID)
        val vaultName = allVault.firstOrNull { it.id == vaultID }?.name ?: vaultID
        val reference = "hisar://$vaultName/${item.title}/$fieldName"
        allCopiedReference += reference
        return reference
    }

    override fun isMacApprovalAllowed(): Boolean = isMacApprovalAllowedValue

    override fun setMacApprovalAllowed(isAllowed: Boolean) {
        record("setMacApprovalAllowed")
        isMacApprovalAllowedValue = isAllowed
    }

    override fun prepareItemIcon(image: ByteString): ByteString {
        record("prepareItemIcon")
        return preparedIcon ?: throw BackendError("choose a PNG, JPEG, GIF or WebP image")
    }

    override fun getWebsiteIcon(url: String): ByteString? {
        record("getWebsiteIcon")
        return if (isWebsiteIconsEnabledValue) websiteIconByURL[url] else null
    }

    override fun isWebsiteIconsEnabled(): Boolean = isWebsiteIconsEnabledValue

    override fun setWebPushEndpoint(endpoint: String, p256dh: String, auth: String) {
        record("setWebPushEndpoint")
        webPushEndpoint = endpoint
        pushSettingsValue = pushSettingsValue.copy(isAvailable = pushSettingsValue.isServerSending)
    }

    override fun removeWebPushEndpoint() {
        record("removeWebPushEndpoint")
        webPushEndpoint = null
        pushSettingsValue = pushSettingsValue.copy(isAvailable = false)
    }

    override fun setPushKindEnabled(kind: PushKind, isEnabled: Boolean) {
        record("setPushKindEnabled")
        val others = pushSettingsValue.enabledKinds - kind.value
        pushSettingsValue = pushSettingsValue.copy(enabledKinds = if (isEnabled) others + kind.value else others)
    }

    override fun getPushSettings(): PushSettings = pushSettingsValue

    override fun setWebsiteIconsEnabled(isEnabled: Boolean) {
        record("setWebsiteIconsEnabled")
        isWebsiteIconsEnabledValue = isEnabled
    }

    override fun getTheme(): VaultTheme = theme

    override fun setTheme(theme: VaultTheme): VaultTheme {
        record("setTheme")
        if (isThemeSaveFailing) throw BackendError("Could not save hisar.json")
        this.theme = theme
        return theme
    }

    // Organizations: the user's organizations with their members and teams; admins change them, members only read.
    override fun listAllOrganization(): Organizations {
        record("listAllOrganization")
        requireUnlocked()
        listAllOrganizationErrorMessage?.let { throw BackendError(it) }
        return Organizations(
            allOrganization = allOrganization.map { OrganizationSummary(it.id, it.name, it.role, it.allMember.size) },
            allInvitation = allOrganizationInvitation.toList(),
        )
    }

    override fun getOrganization(organizationID: String): Organization {
        record("getOrganization")
        requireUnlocked()
        getOrganizationErrorMessage?.let { throw BackendError(it) }
        val organization = allOrganization.firstOrNull { it.id == organizationID } ?: throw BackendError(ORGANIZATION_NOT_FOUND)
        val withVaultNames = organization.copy(
            allTeam = organization.allTeam.map { team ->
                team.copy(
                    allVault = team.allVault.map { teamVault ->
                        val vault = allVault.firstOrNull { it.id == teamVault.vaultID }
                        teamVault.copy(name = vault?.name ?: "", canOpen = vault != null)
                    },
                )
            },
        )
        return if (withVaultNames.isAdmin) withVaultNames else withVaultNames.copy(allInvitation = emptyList())
    }

    override fun createOrganization(name: String): OrganizationSummary {
        record("createOrganization")
        requireUnlocked()
        val currentMember = currentMember
        if (!OrganizationLogic.isValidName(name) || currentMember == null) throw BackendError("Enter a name of 1 to 100 characters")
        val organization = Organization(
            id = "organization-${allOrganization.size + 1}", name = name, role = OrganizationRole.ADMIN, isAdmin = true,
            allMember = listOf(currentMember.copy(role = OrganizationRole.ADMIN)),
        )
        allOrganization += organization
        return OrganizationSummary(organization.id, name, organization.role, 1)
    }

    override fun renameOrganization(organizationID: String, name: String) {
        record("renameOrganization")
        changeOrganizationAsAdmin(organizationID) { it.copy(name = name) }
    }

    override fun deleteOrganization(organizationID: String) {
        record("deleteOrganization")
        changeOrganizationAsAdmin(organizationID) { it }
        if (allVault.any { it.organizationID == organizationID }) throw BackendError("The organization still has vaults; delete them first")
        allOrganization.removeAll { it.id == organizationID }
    }

    override fun inviteToOrganization(organizationID: String, email: String, role: String) {
        record("inviteToOrganization")
        changeOrganizationAsAdmin(organizationID) { organization ->
            val invitation = SentOrganizationInvitation(
                id = "organization-invitation-${organization.allInvitation.size + 1}", email = email, role = role,
                createdAt = "2027-01-15T08:00:00Z", expiresAt = "2027-01-22T08:00:00Z",
            )
            organization.copy(allInvitation = organization.allInvitation + invitation)
        }
    }

    override fun cancelOrganizationInvitation(organizationID: String, invitationID: String) {
        record("cancelOrganizationInvitation")
        changeOrganizationAsAdmin(organizationID) { organization ->
            organization.copy(allInvitation = organization.allInvitation.filter { it.id != invitationID })
        }
    }

    override fun acceptOrganizationInvitation(invitationID: String) {
        record("acceptOrganizationInvitation")
        requireUnlocked()
        val invitation = allOrganizationInvitation.firstOrNull { it.id == invitationID }
        val currentMember = currentMember
        if (invitation == null || currentMember == null) throw BackendError("This invitation no longer exists")
        allOrganizationInvitation.removeAll { it.id == invitationID }
        allOrganization += Organization(
            id = invitation.organizationID, name = invitation.organizationName, role = invitation.role,
            isAdmin = invitation.role == OrganizationRole.ADMIN, allMember = listOf(currentMember.copy(role = invitation.role)),
        )
    }

    override fun declineOrganizationInvitation(invitationID: String) {
        record("declineOrganizationInvitation")
        requireUnlocked()
        if (allOrganizationInvitation.none { it.id == invitationID }) throw BackendError("This invitation no longer exists")
        allOrganizationInvitation.removeAll { it.id == invitationID }
    }

    override fun setOrganizationMemberRole(organizationID: String, accountID: String, role: String) {
        record("setOrganizationMemberRole")
        changeOrganizationAsAdmin(organizationID) { organization ->
            organization.copy(allMember = organization.allMember.map { if (it.accountID == accountID) it.copy(role = role) else it })
        }
    }

    override fun removeOrganizationMember(organizationID: String, accountID: String) {
        record("removeOrganizationMember")
        changeOrganizationAsAdmin(organizationID) { organization ->
            organization.copy(
                allMember = organization.allMember.filter { it.accountID != accountID },
                allTeam = organization.allTeam.map { team -> team.copy(allMember = team.allMember.filter { it.accountID != accountID }) },
            )
        }
    }

    override fun leaveOrganization(organizationID: String) {
        record("leaveOrganization")
        requireUnlocked()
        allOrganization.removeAll { it.id == organizationID }
        allVault.removeAll { it.organizationID == organizationID }
    }

    override fun createTeam(organizationID: String, name: String): Team {
        record("createTeam")
        val team = Team(id = "team-${name.lowercase()}", name = name)
        changeOrganizationAsAdmin(organizationID) { it.copy(allTeam = it.allTeam + team) }
        return team
    }

    override fun renameTeam(organizationID: String, teamID: String, name: String) {
        record("renameTeam")
        changeTeam(organizationID, teamID) { it.copy(name = name) }
    }

    override fun deleteTeam(organizationID: String, teamID: String) {
        record("deleteTeam")
        changeOrganizationAsAdmin(organizationID) { organization -> organization.copy(allTeam = organization.allTeam.filter { it.id != teamID }) }
    }

    override fun addTeamMember(organizationID: String, teamID: String, accountID: String) {
        record("addTeamMember")
        val member = allOrganization.firstOrNull { it.id == organizationID }?.allMember?.firstOrNull { it.accountID == accountID }
            ?: throw BackendError(ORGANIZATION_NOT_FOUND)
        changeTeam(organizationID, teamID) { it.copy(allMember = it.allMember + TeamMember(accountID, member.name, member.email, member.isCurrentUser)) }
    }

    override fun removeTeamMember(organizationID: String, teamID: String, accountID: String) {
        record("removeTeamMember")
        changeTeam(organizationID, teamID) { team -> team.copy(allMember = team.allMember.filter { it.accountID != accountID }) }
    }

    override fun giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String) {
        record("giveVaultToTeam")
        changeTeam(organizationID, teamID) { team ->
            team.copy(allVault = team.allVault.filter { it.vaultID != vaultID } + TeamVault(vaultID, "", role, canOpen = false))
        }
    }

    override fun takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String) {
        record("takeVaultFromTeam")
        changeTeam(organizationID, teamID) { team -> team.copy(allVault = team.allVault.filter { it.vaultID != vaultID }) }
    }

    override fun createOrganizationVault(organizationID: String, teamID: String, name: String): VaultSummary {
        record("createOrganizationVault")
        requireUnlocked()
        val team = allOrganization.firstOrNull { it.id == organizationID }?.allTeam?.firstOrNull { it.id == teamID }
        if (team == null || !team.hasCurrentUser) throw BackendError("Only members of the team can create a vault for it")
        val vault = VaultSummary(
            id = "vault-${allVault.size + 1}", name = name, type = VaultSummary.TYPE_ORGANIZATION, role = Member.VAULT_ROLE_OWNER,
            itemCount = 0, organizationID = organizationID,
        )
        allVault += vault
        allItemByVaultID[vault.id] = mutableListOf()
        changeTeam(organizationID, teamID, isAdminRequired = false) {
            it.copy(allVault = it.allVault + TeamVault(vault.id, "", Member.VAULT_ROLE_OWNER, canOpen = false))
        }
        return vault
    }

    override fun listAllWaitingVaultKey(): WaitingVaultKeys {
        record("listAllWaitingVaultKey")
        requireUnlocked()
        return waitingVaultKeys
    }

    private fun changeOrganizationAsAdmin(organizationID: String, isAdminRequired: Boolean = true, change: (Organization) -> Organization) {
        requireUnlocked()
        organizationRefusal?.let { refusal ->
            organizationRefusal = null
            throw BackendError(refusal)
        }
        val index = allOrganization.indexOfFirst { it.id == organizationID }
        if (index < 0) throw BackendError(ORGANIZATION_NOT_FOUND)
        if (!allOrganization[index].isAdmin && isAdminRequired) throw BackendError("Only an admin of this organization can do that")
        allOrganization[index] = change(allOrganization[index])
    }

    private fun changeTeam(organizationID: String, teamID: String, isAdminRequired: Boolean = true, change: (Team) -> Team) {
        changeOrganizationAsAdmin(organizationID, isAdminRequired) { organization ->
            organization.copy(allTeam = organization.allTeam.map { if (it.id == teamID) change(it) else it })
        }
    }

    override fun addAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail {
        record("addAndroidApp")
        return changeAndroidApps(vaultID, itemID) { it + ItemAndroidApp(packageName, certificateSHA256) }
    }

    override fun removeAndroidApp(vaultID: String, itemID: String, packageName: String, certificateSHA256: String): ItemDetail {
        record("removeAndroidApp")
        return changeAndroidApps(vaultID, itemID) { it - ItemAndroidApp(packageName, certificateSHA256) }
    }

    private fun changeAndroidApps(vaultID: String, itemID: String, change: (List<ItemAndroidApp>) -> List<ItemAndroidApp>): ItemDetail {
        val item = getItem(vaultID, itemID)
        if (!item.offersLoginFill) throw BackendError("only a login or a server can be filled into an app")
        val changed = item.copy(allAndroidApp = change(item.allAndroidApp).distinct())
        if (changed.allAndroidApp != item.allAndroidApp) replaceItem(vaultID, changed)
        return changed
    }

    companion object {
        const val PASSWORD = "correct horse battery"
        const val HISAR_EXPORT_PASSPHRASE = "orange cactus harbor"
        const val SECRET_KEY = "H1-ABC123-AAAAA-BBBBB-CCCCC-DDDDD-EEEEEE"
        const val NEW_ACCOUNT_OPERATION_ID = "new-account-signup"
        const val ONE_TIME_PASSWORD = "otpauth://totp/GitHub:alice?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // gitleaks:allow, the RFC 6238 test secret
        const val SIGN_OUT_OFFLINE_MESSAGE = "Signed out on this device, but the server could not be reached to end the session"
        const val CACHE_REMOVAL_MESSAGE = "The account was deleted, but this device's copy of the vaults could not be removed"
        private const val ORGANIZATION_NOT_FOUND =
            "That organization, team, person or vault was not found, or you are no longer in the organization"
        val oneTimeCode = OneTimeCode(code = "287082", expiresAt = "2027-01-15T08:00:30Z", period = 30)
        val gitHubPasskey = StoredPasskey(
            credentialID = Base64URL.encode(ByteString(1, 1, 1)), relyingPartyID = "github.com", userName = "alice",
            userDisplayName = "Alice", userHandle = Base64URL.encode("alice-handle".encodeToByteString()),
            vaultID = "personal", itemID = "github", title = "GitHub",
        )
    }
}
