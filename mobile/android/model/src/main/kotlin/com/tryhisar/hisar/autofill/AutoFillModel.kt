package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.PasskeyAssertionCredential
import com.tryhisar.hisar.model.PasskeyAssertionRequest
import com.tryhisar.hisar.model.PasskeyRegistrationCredential
import com.tryhisar.hisar.model.PasskeyRegistrationOptions
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.describeError
import kotlinx.io.bytestring.ByteString
import kotlin.coroutines.AbstractCoroutineContextElement
import kotlin.coroutines.CoroutineContext
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.async
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The AutoFill screens' state, the port of the iPhone extension's AutoFillModel for passwords, one-time codes and
 * passkeys. It runs on the app's one backend, so an unlock here unlocks the app. Call it from the main thread; backend
 * calls run on [ioDispatcher].
 */
class AutoFillModel(
    private val scope: CoroutineScope,
    private val ioDispatcher: CoroutineDispatcher,
    private val createBackend: () -> VaultBackend,
    /** The backend's events; a lock, such as auto-lock, while a screen is open shows the lock screen at once. */
    events: Flow<BackendEvent>,
    private val verifier: AppWebsiteVerifier,
    completeRequest: (LoginCredential) -> Unit,
    completeOneTimeCodeRequest: (String) -> Unit,
    copyOneTimeCode: (String) -> Unit,
    private val cancelRequest: (CancelReason) -> Unit,
    passkeyCompletion: PasskeyCompletion,
    private val completeSelection: ((LoginMatch) -> Unit)? = null,
) {
    /** Final passkey operations are verified by the native backend, not sheet continuation extras. */
    class PasskeyCompletion(
        val completeAssertion: (PasskeyAssertionCredential) -> Unit,
        val completeRegistration: (PasskeyRegistrationCredential) -> Unit,
        val rejectExcludedRegistration: () -> Unit,
        val completeUnlock: () -> Unit,
    )

    /** Why a request ends without a credential. */
    enum class CancelReason {
        USER_CANCELED,

        /** A passkey request Hisar cannot answer, such as a create without ES256. */
        NOT_SUPPORTED,

        /** The passkey picked in the system's sheet is no longer in Hisar. */
        CREDENTIAL_NOT_FOUND,
    }

    sealed interface Screen {
        data object Starting : Screen
        data class Failed(val message: String) : Screen
        data object SignedOut : Screen
        data object Locked : Screen
        data object List : Screen
        data object PasskeyRegistration : Screen
        data object UpdateRequired : Screen
    }

    sealed interface MatchingLookup {
        data object Loading : MatchingLookup
        data object Loaded : MatchingLookup
        data class Failed(val message: String) : MatchingLookup
    }

    private val mutableScreen = MutableStateFlow<Screen>(Screen.Starting)
    private val mutableStatus = MutableStateFlow(VaultStatus.signedOut)
    private val mutableIsBusy = MutableStateFlow(false)
    private val mutableAllMatchingLogin = MutableStateFlow<List<LoginMatch>>(emptyList())
    private val mutableMatchingLookup = MutableStateFlow<MatchingLookup>(MatchingLookup.Loading)
    private val mutableAllOtherLogin = MutableStateFlow<List<LoginMatch>>(emptyList())
    private val mutableIsOtherLoginLoaded = MutableStateFlow(false)
    private val mutableSearchText = MutableStateFlow("")
    private val mutableLockCount = MutableStateFlow(0)
    private val mutableRequestKind = MutableStateFlow(RequestKind.PASSWORD)
    private val mutableTheme = MutableStateFlow(VaultTheme.standard)
    private val mutableTarget = MutableStateFlow<FillTarget?>(null)
    private val mutableLoginToLink = MutableStateFlow<LoginMatch?>(null)
    private val mutableAllMatchingPasskey = MutableStateFlow<List<StoredPasskey>>(emptyList())
    private val mutablePasskeyAssertionRequest = MutableStateFlow<PasskeyAssertionRequest?>(null)
    private val mutablePasskeyRegistrationRequest = MutableStateFlow<PasskeyRegistrationRequest?>(null)
    private val mutablePasskeyRegistrationOptions = MutableStateFlow<PasskeyRegistrationOptions?>(null)

    val screen: StateFlow<Screen> = mutableScreen.asStateFlow()
    val status: StateFlow<VaultStatus> = mutableStatus.asStateFlow()
    val isBusy: StateFlow<Boolean> = mutableIsBusy.asStateFlow()
    val errorMessage = MutableStateFlow<String?>(null)
    val allMatchingLogin: StateFlow<List<LoginMatch>> = mutableAllMatchingLogin.asStateFlow()
    val matchingLookup: StateFlow<MatchingLookup> = mutableMatchingLookup.asStateFlow()
    val allOtherLogin: StateFlow<List<LoginMatch>> = mutableAllOtherLogin.asStateFlow()
    val isOtherLoginLoaded: StateFlow<Boolean> = mutableIsOtherLoginLoaded.asStateFlow()
    val searchText: StateFlow<String> = mutableSearchText.asStateFlow()

    /** Grows with every lock; the lock screen offers biometrics once per value. */
    val lockCount: StateFlow<Int> = mutableLockCount.asStateFlow()
    val requestKind: StateFlow<RequestKind> = mutableRequestKind.asStateFlow()

    /** The app's theme, which AutoFill follows. */
    val theme: StateFlow<VaultTheme> = mutableTheme.asStateFlow()
    val target: StateFlow<FillTarget?> = mutableTarget.asStateFlow()

    /** A login found by search for an app it is not linked to, waiting for the user to link it or fill it once. */
    val loginToLink: StateFlow<LoginMatch?> = mutableLoginToLink.asStateFlow()
    val allMatchingPasskey: StateFlow<List<StoredPasskey>> = mutableAllMatchingPasskey.asStateFlow()
    val passkeyAssertionRequest: StateFlow<PasskeyAssertionRequest?> = mutablePasskeyAssertionRequest.asStateFlow()
    val passkeyRegistrationRequest: StateFlow<PasskeyRegistrationRequest?> = mutablePasskeyRegistrationRequest.asStateFlow()
    val passkeyRegistrationOptions: StateFlow<PasskeyRegistrationOptions?> = mutablePasskeyRegistrationOptions.asStateFlow()
    val selectedRegistrationLoginID = MutableStateFlow<String?>(null)

    private var pendingRequest: PendingCredentialRequest? = null
    private var pendingPasskeyCredentialID: ByteString? = null
    private var requestEpoch: Long = 0


    // The sheet's "Unlock Hisar" action only unlocks; the sheet then asks again and lists the credentials itself.
    private var isUnlockOnly = false
    private var sourceByLoginID: Map<String, LoginSource> = emptyMap()
    private var backend: VaultBackend? = null

    private class UnlockGeneration(val count: Int) : AbstractCoroutineContextElement(Key) {
        companion object Key : CoroutineContext.Key<UnlockGeneration>
    }

    private val completeRequest: suspend (LoginCredential) -> Unit = { value -> completeWithinUnlock { completeRequest(value) } }
    private val completeOneTimeCodeRequest: suspend (String) -> Unit = { value -> completeWithinUnlock { completeOneTimeCodeRequest(value) } }
    private val copyOneTimeCode: suspend (String) -> Unit = { value -> completeWithinUnlock { copyOneTimeCode(value) } }
    private val completeAssertion: suspend (PasskeyAssertionCredential) -> Unit = { value -> completeWithinUnlock { passkeyCompletion.completeAssertion(value) } }
    private val completeUnlock: suspend () -> Unit = { completeWithinUnlock { passkeyCompletion.completeUnlock() } }
    private val completeRegistration = passkeyCompletion.completeRegistration
    private val rejectExcludedRegistration = passkeyCompletion.rejectExcludedRegistration

    private suspend fun completeWithinUnlock(complete: () -> Unit) {
        val generation = checkNotNull(currentCoroutineContext()[UnlockGeneration])
        if (generation.count == mutableLockCount.value) complete()
    }

    init {
        scope.launch(start = CoroutineStart.UNDISPATCHED) {
            events.collect { event ->
                if (event != BackendEvent.Locked && event != BackendEvent.UpdateRequired) return@collect
                if (!mutableStatus.value.isUnlocked) mutableLockCount.update { it + 1 }
                forgetUnlockedData()
                applyStatus(mutableStatus.value.copy(state = VaultStatus.STATE_LOCKED))
                val backend = backend ?: return@collect
                try {
                    refreshStatus(backend)
                } catch (error: CancellationException) {
                    throw error
                } catch (_: Exception) {
                }
            }
        }
    }

    val serviceName: String
        get() = mutableTarget.value?.name?.ifEmpty { null }
            ?: mutablePasskeyAssertionRequest.value?.relyingPartyID
            ?: mutablePasskeyRegistrationRequest.value?.relyingPartyID
            ?: ""

    val allVisibleMatchingPasskey: List<StoredPasskey>
        get() {
            val allWord = mutableSearchText.value.lowercase().split(Regex("\\s+")).filter { it.isNotEmpty() }
            return mutableAllMatchingPasskey.value.filter { passkey ->
                val searchable = "${passkey.title} ${passkey.userName} ${passkey.userDisplayName}".lowercase()
                allWord.all { it in searchable }
            }
        }

    val allVisibleMatchingLogin: List<LoginMatch>
        get() = filterAllLogin(filterForRequest(mutableAllMatchingLogin.value), mutableSearchText.value)

    val allVisibleOtherLogin: List<LoginMatch>
        get() {
            val searchText = mutableSearchText.value
            if (searchText.isEmpty()) return emptyList()
            val allMatchingID = mutableAllMatchingLogin.value.map { it.id }.toSet()
            return filterAllLogin(filterForRequest(mutableAllOtherLogin.value).filter { it.id !in allMatchingID }, searchText)
        }

    private fun filterForRequest(allLogin: List<LoginMatch>): List<LoginMatch> =
        if (mutableRequestKind.value == RequestKind.ONE_TIME_CODE) allLogin.filter { it.hasOneTimePassword } else allLogin

    /** Typing a search loads every fillable login once, for the "All logins" section. */
    fun updateSearchText(text: String) {
        mutableSearchText.value = text
        if (text.isNotEmpty()) scope.launch { loadAllOtherLogin() }
    }

    /** Builds the backend once and reads the app's theme; a backend that cannot start shows why. */
    suspend fun prepare(): VaultBackend? {
        backend?.let { return it }
        val lockCount = mutableLockCount.value
        return try {
            val (backend, theme) = withContext(ioDispatcher) { createBackend().let { it to it.getTheme() } }
            if (mutableLockCount.value != lockCount) return null
            this.backend = backend
            mutableTheme.value = theme
            backend
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (mutableLockCount.value == lockCount) mutableScreen.value = Screen.Failed(describeError(error))
            null
        }
    }

    private suspend fun <Value> performWithinUnlock(operation: (VaultBackend) -> Value): Value? {
        val lockCount = currentCoroutineContext()[UnlockGeneration]?.count ?: mutableLockCount.value
        if (mutableLockCount.value != lockCount) return null
        val backend = prepare() ?: return null
        mutableIsBusy.value = true
        return try {
            val value = withContext(ioDispatcher) { operation(backend) }
            if (mutableLockCount.value != lockCount) return null
            errorMessage.value = null
            value
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (mutableLockCount.value == lockCount) {
                if ((error as? BackendError)?.code == BackendError.PASSKEY_VERIFICATION_FAILED) {
                    cancelRequest(CancelReason.USER_CANCELED)
                } else {
                    errorMessage.value = describeError(error)
                }
            }
            null
        } finally {
            mutableIsBusy.value = false
        }
    }

    /** Carries one lock generation through suspended work and its completion sinks. */
    suspend fun <Value> withinUnlock(lockCount: Int = mutableLockCount.value, operation: suspend () -> Value): Value? {
        val generation = currentCoroutineContext()[UnlockGeneration] ?: UnlockGeneration(lockCount)
        if (mutableLockCount.value != generation.count) return null
        val value = withContext(generation) { operation() }
        return value.takeIf { mutableLockCount.value == generation.count }
    }

    private fun matcher(backend: VaultBackend) = LoginMatcher(backend, verifier)

    private suspend fun refreshStatus(backend: VaultBackend): Boolean {
        val newStatus = withinUnlock { withContext(ioDispatcher) { backend.getStatus() } } ?: return false
        applyStatus(newStatus)
        return true
    }

    private fun applyStatus(newStatus: VaultStatus) {
        if (mutableStatus.value.isUnlocked && !newStatus.isUnlocked) {
            mutableLockCount.update { it + 1 }
            forgetUnlockedData()
        }
        mutableStatus.value = newStatus
        mutableScreen.value = when {
            newStatus.updateRequired != null -> Screen.UpdateRequired
            newStatus.state == VaultStatus.STATE_UNLOCKED -> when {
                isUnlockOnly -> Screen.Starting
                mutablePasskeyRegistrationRequest.value != null -> Screen.PasskeyRegistration
                else -> Screen.List
            }
            newStatus.state == VaultStatus.STATE_LOCKED -> Screen.Locked
            else -> Screen.SignedOut
        }
    }

    // What only an unlocked Hisar shows; the unlock loads it again for the request.
    private fun forgetUnlockedData() {
        mutableAllMatchingLogin.value = emptyList()
        mutableMatchingLookup.value = MatchingLookup.Loading
        mutableAllOtherLogin.value = emptyList()
        mutableIsOtherLoginLoaded.value = false
        mutableSearchText.value = ""
        mutableAllMatchingPasskey.value = emptyList()
        mutablePasskeyRegistrationOptions.value = null
        mutableLoginToLink.value = null
        selectedRegistrationLoginID.value = null
        sourceByLoginID = emptyMap()
    }

    private fun resetRequest(target: FillTarget, kind: RequestKind) {
        endPasskeyCeremony()
        forgetUnlockedData()
        pendingRequest = null
        pendingPasskeyCredentialID = null
        isUnlockOnly = false
        mutableLoginToLink.value = null
        mutableTarget.value = target
        mutableRequestKind.value = kind
        mutablePasskeyAssertionRequest.value = null
        mutablePasskeyRegistrationRequest.value = null
        mutablePasskeyRegistrationOptions.value = null
        selectedRegistrationLoginID.value = null
        mutableAllMatchingPasskey.value = emptyList()
    }

    /** The list of logins for a site or app, and of passkeys for a passkey request, after an unlock when Hisar is locked. */
    suspend fun prepareCredentialList(target: FillTarget, kind: RequestKind = RequestKind.PASSWORD, passkeyRequest: PasskeyAssertionRequest? = null) {
        resetRequest(target, kind)
        mutablePasskeyAssertionRequest.value = passkeyRequest
        val backend = prepare() ?: return
        if (!refreshStatus(backend)) return
        if (mutableStatus.value.isUnlocked) loadAllMatchingCredential()
    }

    /** The sheet's "Unlock Hisar" action: once Hisar is unlocked, hands back so the sheet can ask for the credentials again. */
    suspend fun prepareUnlock(target: FillTarget, passkeyRequest: PasskeyAssertionRequest?) {
        resetRequest(target, RequestKind.PASSWORD)
        mutablePasskeyAssertionRequest.value = passkeyRequest
        isUnlockOnly = true
        val backend = prepare() ?: return
        if (!refreshStatus(backend)) return
        if (mutableStatus.value.isUnlocked) withinUnlock { completeUnlock() }
    }

    /** A suggestion the user picked: filled at once when unlocked, else after the unlock. */
    suspend fun prepareInterfaceToProvideCredential(
        recordIdentifier: String,
        target: FillTarget,
        source: LoginSource,
        kind: RequestKind = RequestKind.PASSWORD,
    ) {
        resetRequest(target, kind)
        pendingRequest = PendingCredentialRequest(recordIdentifier, source, kind)
        val backend = prepare() ?: return
        if (!refreshStatus(backend)) return
        if (mutableStatus.value.isUnlocked) finishPendingRequest()
    }

    // Without user interaction only a backend that is prepared and still unlocked can answer.
    suspend fun provideCredentialWithoutUserInteraction(recordIdentifier: String, source: LoginSource?): LoginCredential? = withinUnlock {
        val backend = backend ?: return@withinUnlock null
        val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(recordIdentifier) ?: return@withinUnlock null
        val credential = withinUnlock {
            withContext(ioDispatcher) {
                val status = backend.getStatus()
                if (!status.isUnlocked || status.updateRequired != null) return@withContext null
                runCatching { matcher(backend).getLogin(vaultID, itemID, source) }.getOrNull()
            }
        } ?: return@withinUnlock null
        if (credential.hasOneTimePassword) copyCode(vaultID, itemID, source)
        credential
    }

    suspend fun provideOneTimeCodeWithoutUserInteraction(recordIdentifier: String, source: LoginSource): String? = withinUnlock {
        val backend = backend ?: return@withinUnlock null
        val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(recordIdentifier) ?: return@withinUnlock null
        withContext(ioDispatcher) {
            val status = backend.getStatus()
            if (!status.isUnlocked || status.updateRequired != null) return@withContext null
            runCatching { matcher(backend).getOneTimeCode(vaultID, itemID, source).code }.getOrNull()
        }
    }

    // The code goes to the clipboard after a password fill, for the site's next step; a failure never stops the fill.
    private suspend fun copyCode(vaultID: String, itemID: String, source: LoginSource?) = withinUnlock {
        val backend = backend ?: return@withinUnlock
        val code = withinUnlock {
            withContext(ioDispatcher) { runCatching { matcher(backend).getOneTimeCode(vaultID, itemID, source) }.getOrNull() }
        }
        if (code != null) copyOneTimeCode(code.code)
    }

    // Unlocking replaces the lock screen that called it, so the work runs in the model's scope, not the screen's.
    suspend fun unlock(password: String): Boolean = scope.async {
        withinUnlock {
            val newStatus = performWithinUnlock { it.unlock(password) } ?: return@withinUnlock false
            handleUnlocked(newStatus)
            true
        } ?: false
    }.await()

    suspend fun unlockWithBiometric() = scope.async {
        withinUnlock {
            val newStatus = if (mutableStatus.value.biometricReady) performWithinUnlock { it.unlockWithBiometric() } else null
            if (newStatus == null) {
                backend?.let { refreshStatus(it) }
            } else {
                handleUnlocked(newStatus)
            }
        }
    }.await()

    private suspend fun handleUnlocked(newStatus: VaultStatus) {
        applyStatus(newStatus)
        if (!newStatus.isUnlocked) return
        when {
            isUnlockOnly -> completeUnlock()
            mutablePasskeyRegistrationRequest.value != null -> loadPasskeyRegistrationOptions()
            pendingRequest != null || pendingPasskeyCredentialID != null -> finishPendingRequest()
            else -> loadAllMatchingCredential()
        }
    }

    /** A concrete system-sheet selection; the backend verifies this final request before signing. */
    suspend fun prepareInterfaceToProvidePasskey(request: PasskeyAssertionRequest, credentialID: ByteString) {
        resetRequest(FillTarget.Websites(emptyList()), RequestKind.PASSWORD)
        mutablePasskeyAssertionRequest.value = request
        pendingPasskeyCredentialID = credentialID
        val backend = prepare() ?: return
        if (!refreshStatus(backend)) return
        if (mutableStatus.value.isUnlocked) finishPendingRequest()
    }

    /** Discouraged verification can use the system sheet's presence confirmation without another prompt. */
    suspend fun providePasskeyWithoutUserInteraction(request: PasskeyAssertionRequest, credentialID: ByteString): PasskeyAssertionCredential? = withinUnlock {
        val epoch = requestEpoch
        if (!request.isUserVerificationDiscouraged) return@withinUnlock null
        val backend = backend ?: return@withinUnlock null
        val result = withContext(ioDispatcher) {
            val status = backend.getStatus()
            if (!status.isUnlocked || status.updateRequired != null) return@withContext null
            runCatching { backend.assertPasskey(request.relyingPartyID, credentialID, request.clientDataHash, request.userVerification) }.getOrNull()
        } ?: return@withinUnlock null
        if (epoch != requestEpoch) return@withinUnlock null
        PasskeyAssertionCredential.create(request.relyingPartyID, request.clientDataHash, result)
    }

    /** A site asks to save a passkey; without ES256, which Hisar's passkeys use, the request fails. */
    suspend fun prepareInterfaceForPasskeyRegistration(request: PasskeyRegistrationRequest) {
        resetRequest(FillTarget.Websites(emptyList()), RequestKind.PASSWORD)
        if (!request.isAlgorithmSupported) return cancelRequest(CancelReason.NOT_SUPPORTED)
        mutablePasskeyRegistrationRequest.value = request
        val backend = prepare() ?: return
        if (!refreshStatus(backend)) return
        if (mutableStatus.value.isUnlocked) loadPasskeyRegistrationOptions()
    }

    /** The logins the passkey can join, with the one whose username is the account's user name chosen. */
    suspend fun loadPasskeyRegistrationOptions() {
        val request = mutablePasskeyRegistrationRequest.value ?: return
        val options = performWithinUnlock { it.preparePasskeyRegistration(request.relyingPartyID, request.allExcludedCredentialID) } ?: return
        mutablePasskeyRegistrationOptions.value = options
        val userName = request.userName.trim()
        selectedRegistrationLoginID.value = options.allLogin.firstOrNull { it.isPersonal && userName.isNotEmpty() && it.username.equals(userName, ignoreCase = true) }?.id
    }

    /** Saves the passkey to the chosen login, or to a new login in the Personal vault, once the device owner confirms. */
    suspend fun savePasskey() {
        withinUnlock {
            val epoch = requestEpoch
            val request = mutablePasskeyRegistrationRequest.value ?: return@withinUnlock
            val options = mutablePasskeyRegistrationOptions.value?.takeIf { !it.isExcluded } ?: return@withinUnlock
            val login = options.allLogin.firstOrNull { it.id == selectedRegistrationLoginID.value }
            val backend = backend ?: return@withinUnlock
            mutableIsBusy.value = true
            try {
                // Go commits before returning. Its public registration response must reach the site even after a lock.
                val result = withContext(ioDispatcher) { backend.createPasskey(request, login?.vaultID.orEmpty(), login?.itemID.orEmpty()) }
                if (epoch != requestEpoch) return@withinUnlock
                val credential = PasskeyRegistrationCredential.create(request.relyingPartyID, request.clientDataHash, result)
                if (credential == null) {
                    errorMessage.value = "Hisar could not read the new passkey."
                } else {
                    completeRegistration(credential)
                }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                if (epoch == requestEpoch && currentCoroutineContext()[UnlockGeneration]?.count == mutableLockCount.value) {
                    if ((error as? BackendError)?.code == BackendError.PASSKEY_VERIFICATION_FAILED) {
                        cancelRequest(CancelReason.USER_CANCELED)
                    } else {
                        errorMessage.value = describeError(error)
                    }
                }
            } finally {
                mutableIsBusy.value = false
            }
        }
    }

    fun rejectExcludedPasskeyRegistration() {
        rejectExcludedRegistration()
    }

    suspend fun selectPasskey(passkey: StoredPasskey) {
        withinUnlock {
            val request = mutablePasskeyAssertionRequest.value ?: return@withinUnlock
            val credentialID = Base64URL.decode(passkey.credentialID) ?: return@withinUnlock
            assertPasskey(request, credentialID)
        }
    }


    private suspend fun assertPasskey(request: PasskeyAssertionRequest, credentialID: ByteString): Boolean {
        val epoch = requestEpoch
        val result = performWithinUnlock { it.assertPasskey(request.relyingPartyID, credentialID, request.clientDataHash, request.userVerification) } ?: return false
        if (epoch != requestEpoch) return false
        val credential = PasskeyAssertionCredential.create(request.relyingPartyID, request.clientDataHash, result)
        if (credential == null) {
            errorMessage.value = "Hisar could not read the passkey signature."
            return false
        }
        completeAssertion(credential)
        return true
    }

    // A passkey whose login no longer matches falls back to the list; one Hisar no longer has fails the request. A
    // suggestion whose login is gone falls back to the list.
    private suspend fun finishPendingRequest() {
        withinUnlock {
            val credentialID = pendingPasskeyCredentialID
            val passkeyRequest = mutablePasskeyAssertionRequest.value
            if (credentialID != null && passkeyRequest != null) {
                val allFound = performWithinUnlock { it.listAllPasskeyForRelyingParty(passkeyRequest.relyingPartyID, listOf(credentialID)) } ?: return@withinUnlock
                if (allFound.isEmpty()) {
                    cancelRequest(CancelReason.CREDENTIAL_NOT_FOUND)
                    return@withinUnlock
                }
                if (assertPasskey(passkeyRequest, credentialID)) return@withinUnlock
                pendingPasskeyCredentialID = null
                loadAllMatchingCredential()
                return@withinUnlock
            }
            val request = pendingRequest ?: return@withinUnlock
            val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(request.recordIdentifier) ?: return@withinUnlock cancel()
            if (request.kind == RequestKind.ONE_TIME_CODE) {
                performWithinUnlock { matcher(it).getOneTimeCode(vaultID, itemID, request.source) }?.let { code ->
                    completeOneTimeCodeRequest(code.code)
                    return@withinUnlock
                }
            } else {
                performWithinUnlock { matcher(it).getLogin(vaultID, itemID, request.source) }?.let { credential ->
                    if (credential.hasOneTimePassword) copyCode(vaultID, itemID, request.source)
                    completeRequest(credential)
                    return@withinUnlock
                }
            }
            pendingRequest = null
            loadAllMatchingLogin()
        }
    }

    suspend fun loadAllMatchingCredential() {
        loadAllMatchingPasskey()
        loadAllMatchingLogin()
    }

    suspend fun loadAllMatchingPasskey() {
        val request = mutablePasskeyAssertionRequest.value ?: return run { mutableAllMatchingPasskey.value = emptyList() }
        val allPasskey = performWithinUnlock { it.listAllPasskeyForRelyingParty(request.relyingPartyID, request.allAllowedCredentialID) } ?: return
        mutableAllMatchingPasskey.value = allPasskey
    }

    suspend fun loadAllMatchingLogin() {
        val target = mutableTarget.value ?: return
        val epoch = requestEpoch
        val lockCount = mutableLockCount.value
        mutableMatchingLookup.value = MatchingLookup.Loading
        val allMatch = performWithinUnlock { matcher(it).listAllMatch(target) }
        if (epoch != requestEpoch || lockCount != mutableLockCount.value) return
        if (allMatch == null) {
            mutableMatchingLookup.value = MatchingLookup.Failed(errorMessage.value ?: "Could not look for logins. Try again.")
            errorMessage.value = null
            return
        }
        sourceByLoginID = allMatch.associate { it.login.id to it.source }
        mutableAllMatchingLogin.value = allMatch.map { it.login }
        mutableMatchingLookup.value = MatchingLookup.Loaded
    }

    suspend fun loadAllOtherLogin() {
        if (mutableIsOtherLoginLoaded.value || !mutableStatus.value.isUnlocked || mutableIsBusy.value) return
        val allLogin = performWithinUnlock { backend ->
            backend.listAllVault().flatMap { vault ->
                backend.listAllItem(vault.id).filter { it.offersLoginFill }.map { item ->
                    LoginMatch(vault.id, item.id, item.title, item.username, vault.name, item.hasOneTimePassword)
                }
            }.sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.title })
        } ?: return
        mutableAllOtherLogin.value = allLogin
        mutableIsOtherLoginLoaded.value = true
    }

    /** Fills the login. A login search found for an app it is not linked to first asks whether to link it. */
    suspend fun select(login: LoginMatch) {
        val source = sourceByLoginID[login.id]
        if (source == null && mutableTarget.value is FillTarget.App) {
            mutableLoginToLink.value = login
            return
        }
        fill(login, source)
    }

    /** Links the login to the app ("Always use for this app") and fills it, or fills it once. */
    suspend fun fillLoginToLink(isLinked: Boolean) {
        val login = mutableLoginToLink.value ?: return
        val app = (mutableTarget.value as? FillTarget.App)?.app ?: return
        mutableLoginToLink.value = null
        if (!isLinked) return fill(login, null)
        performWithinUnlock { it.addAndroidApp(login.vaultID, login.itemID, app.packageName, app.certificateSHA256) } ?: return
        fill(login, LoginSource.App(app))
    }

    fun dismissLoginToLink() {
        mutableLoginToLink.value = null
    }

    private suspend fun fill(login: LoginMatch, source: LoginSource?) {
        withinUnlock {
            if (mutableRequestKind.value == RequestKind.ONE_TIME_CODE) {
                performWithinUnlock { matcher(it).getOneTimeCode(login.vaultID, login.itemID, source) }?.let { completeOneTimeCodeRequest(it.code) }
                return@withinUnlock
            }
            if (completeSelection != null) {
                completeWithinUnlock { completeSelection(login) }
                return@withinUnlock
            }
            val credential = performWithinUnlock { matcher(it).getLogin(login.vaultID, login.itemID, source) } ?: return@withinUnlock
            if (credential.hasOneTimePassword) copyCode(login.vaultID, login.itemID, source)
            completeRequest(credential)
        }
    }

    fun endPasskeyCeremony() {
        requestEpoch++
        backend?.cancelPasskeyCeremonies()
    }

    fun cancel() {
        endPasskeyCeremony()
        cancelRequest(CancelReason.USER_CANCELED)
    }

    companion object {
        /** The logins whose title, username or vault contain every word of the search. */
        fun filterAllLogin(allLogin: List<LoginMatch>, searchText: String): List<LoginMatch> {
            val allWord = searchText.lowercase().split(Regex("\\s+")).filter { it.isNotEmpty() }
            if (allWord.isEmpty()) return allLogin
            return allLogin.filter { login ->
                val searchable = "${login.title} ${login.username} ${login.vaultName}".lowercase()
                allWord.all { it in searchable }
            }
        }
    }
}
