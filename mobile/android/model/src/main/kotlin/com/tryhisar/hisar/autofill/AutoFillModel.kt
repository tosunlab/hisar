package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.describeError
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The AutoFill screens' state, the port of the iPhone extension's AutoFillModel for passwords and one-time codes. It runs
 * on the app's one backend, so an unlock here unlocks the app. Call it from the main thread; backend calls run on
 * [ioDispatcher].
 */
class AutoFillModel(
    private val scope: CoroutineScope,
    private val ioDispatcher: CoroutineDispatcher,
    private val createBackend: () -> VaultBackend,
    private val verifier: AppWebsiteVerifier,
    private val completeRequest: (LoginCredential) -> Unit,
    private val completeOneTimeCodeRequest: (String) -> Unit,
    private val copyOneTimeCode: (String) -> Unit,
    private val cancelRequest: () -> Unit,
) {
    sealed interface Screen {
        data object Starting : Screen
        data class Failed(val message: String) : Screen
        data object SignedOut : Screen
        data object Locked : Screen
        data object List : Screen
        data object UpdateRequired : Screen
    }

    private val mutableScreen = MutableStateFlow<Screen>(Screen.Starting)
    private val mutableStatus = MutableStateFlow(VaultStatus.signedOut)
    private val mutableIsBusy = MutableStateFlow(false)
    private val mutableAllMatchingLogin = MutableStateFlow<List<LoginMatch>>(emptyList())
    private val mutableAllOtherLogin = MutableStateFlow<List<LoginMatch>>(emptyList())
    private val mutableIsOtherLoginLoaded = MutableStateFlow(false)
    private val mutableSearchText = MutableStateFlow("")
    private val mutableLockCount = MutableStateFlow(0)
    private val mutableRequestKind = MutableStateFlow(RequestKind.PASSWORD)
    private val mutableTheme = MutableStateFlow(VaultTheme.standard)
    private val mutableTarget = MutableStateFlow<FillTarget?>(null)
    private val mutableLoginToLink = MutableStateFlow<LoginMatch?>(null)

    val screen: StateFlow<Screen> = mutableScreen.asStateFlow()
    val status: StateFlow<VaultStatus> = mutableStatus.asStateFlow()
    val isBusy: StateFlow<Boolean> = mutableIsBusy.asStateFlow()
    val errorMessage = MutableStateFlow<String?>(null)
    val allMatchingLogin: StateFlow<List<LoginMatch>> = mutableAllMatchingLogin.asStateFlow()
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

    private var pendingRequest: PendingCredentialRequest? = null
    private var sourceByLoginID: Map<String, LoginSource> = emptyMap()
    private var backend: VaultBackend? = null

    val serviceName: String get() = mutableTarget.value?.name.orEmpty()

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
        return try {
            val (backend, theme) = withContext(ioDispatcher) { createBackend().let { it to it.getTheme() } }
            this.backend = backend
            mutableTheme.value = theme
            backend
        } catch (error: Exception) {
            mutableScreen.value = Screen.Failed(describeError(error))
            null
        }
    }

    private suspend fun <Value> perform(operation: (VaultBackend) -> Value): Value? {
        val backend = prepare() ?: return null
        mutableIsBusy.value = true
        return try {
            val value = withContext(ioDispatcher) { operation(backend) }
            errorMessage.value = null
            value
        } catch (error: Exception) {
            errorMessage.value = describeError(error)
            null
        } finally {
            mutableIsBusy.value = false
        }
    }

    private fun matcher(backend: VaultBackend) = LoginMatcher(backend, verifier)

    private suspend fun readStatus(backend: VaultBackend): VaultStatus = withContext(ioDispatcher) { backend.getStatus() }

    private fun applyStatus(newStatus: VaultStatus) {
        if (mutableStatus.value.isUnlocked && !newStatus.isUnlocked) mutableLockCount.update { it + 1 }
        mutableStatus.value = newStatus
        mutableScreen.value = when {
            newStatus.updateRequired != null -> Screen.UpdateRequired
            newStatus.state == VaultStatus.STATE_UNLOCKED -> Screen.List
            newStatus.state == VaultStatus.STATE_LOCKED -> Screen.Locked
            else -> Screen.SignedOut
        }
    }

    private fun resetRequest(target: FillTarget, kind: RequestKind) {
        pendingRequest = null
        mutableLoginToLink.value = null
        mutableTarget.value = target
        mutableRequestKind.value = kind
    }

    /** The list of logins for a site or app, after an unlock when Hisar is locked. */
    suspend fun prepareCredentialList(target: FillTarget, kind: RequestKind = RequestKind.PASSWORD) {
        resetRequest(target, kind)
        val backend = prepare() ?: return
        applyStatus(readStatus(backend))
        if (mutableStatus.value.isUnlocked) loadAllMatchingLogin()
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
        applyStatus(readStatus(backend))
        if (mutableStatus.value.isUnlocked) finishPendingRequest()
    }

    // Without user interaction only a backend that is prepared and still unlocked can answer.
    suspend fun provideCredentialWithoutUserInteraction(recordIdentifier: String, source: LoginSource): LoginCredential? {
        val backend = backend ?: return null
        val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(recordIdentifier) ?: return null
        val credential = withContext(ioDispatcher) {
            val status = backend.getStatus()
            if (!status.isUnlocked || status.updateRequired != null) return@withContext null
            runCatching { matcher(backend).getLogin(vaultID, itemID, source) }.getOrNull()
        } ?: return null
        if (credential.hasOneTimePassword) copyCode(vaultID, itemID, source)
        return credential
    }

    suspend fun provideOneTimeCodeWithoutUserInteraction(recordIdentifier: String, source: LoginSource): String? {
        val backend = backend ?: return null
        val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(recordIdentifier) ?: return null
        return withContext(ioDispatcher) {
            val status = backend.getStatus()
            if (!status.isUnlocked || status.updateRequired != null) return@withContext null
            runCatching { matcher(backend).getOneTimeCode(vaultID, itemID, source).code }.getOrNull()
        }
    }

    // The code goes to the clipboard after a password fill, for the site's next step; a failure never stops the fill.
    private suspend fun copyCode(vaultID: String, itemID: String, source: LoginSource?) {
        val backend = backend ?: return
        val code = withContext(ioDispatcher) { runCatching { matcher(backend).getOneTimeCode(vaultID, itemID, source) }.getOrNull() }
        if (code != null) copyOneTimeCode(code.code)
    }

    suspend fun unlock(password: String): Boolean {
        val newStatus = perform { it.unlock(password) } ?: return false
        handleUnlocked(newStatus)
        return true
    }

    suspend fun unlockWithBiometric() {
        val newStatus = if (mutableStatus.value.biometricReady) perform { it.unlockWithBiometric() } else null
        if (newStatus == null) {
            backend?.let { mutableStatus.value = readStatus(it) }
            return
        }
        handleUnlocked(newStatus)
    }

    private suspend fun handleUnlocked(newStatus: VaultStatus) {
        applyStatus(newStatus)
        if (!newStatus.isUnlocked) return
        if (pendingRequest != null) finishPendingRequest() else loadAllMatchingLogin()
    }

    // A suggestion that no longer matches, or whose login is gone, falls back to the list.
    private suspend fun finishPendingRequest() {
        val request = pendingRequest ?: return
        val (vaultID, itemID) = LoginMatch.parseRecordIdentifier(request.recordIdentifier) ?: return cancel()
        if (request.kind == RequestKind.ONE_TIME_CODE) {
            perform { matcher(it).getOneTimeCode(vaultID, itemID, request.source) }?.let { code ->
                completeOneTimeCodeRequest(code.code)
                return
            }
        } else {
            perform { matcher(it).getLogin(vaultID, itemID, request.source) }?.let { credential ->
                if (credential.hasOneTimePassword) copyCode(vaultID, itemID, request.source)
                completeRequest(credential)
                return
            }
        }
        pendingRequest = null
        loadAllMatchingLogin()
    }

    suspend fun loadAllMatchingLogin() {
        val target = mutableTarget.value ?: return
        val allMatch = perform { matcher(it).listAllMatch(target) } ?: return
        sourceByLoginID = allMatch.associate { it.login.id to it.source }
        mutableAllMatchingLogin.value = allMatch.map { it.login }
    }

    suspend fun loadAllOtherLogin() {
        if (mutableIsOtherLoginLoaded.value || !mutableStatus.value.isUnlocked || mutableIsBusy.value) return
        val allLogin = perform { backend ->
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
        perform { it.addAndroidApp(login.vaultID, login.itemID, app.packageName, app.certificateSHA256) } ?: return
        fill(login, LoginSource.App(app))
    }

    fun dismissLoginToLink() {
        mutableLoginToLink.value = null
    }

    private suspend fun fill(login: LoginMatch, source: LoginSource?) {
        if (mutableRequestKind.value == RequestKind.ONE_TIME_CODE) {
            perform { matcher(it).getOneTimeCode(login.vaultID, login.itemID, source) }?.let { completeOneTimeCodeRequest(it.code) }
            return
        }
        val credential = perform { matcher(it).getLogin(login.vaultID, login.itemID, source) } ?: return
        if (credential.hasOneTimePassword) copyCode(login.vaultID, login.itemID, source)
        completeRequest(credential)
    }

    fun cancel() {
        cancelRequest()
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
