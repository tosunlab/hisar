package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.model.PasskeyAssertionCredential
import com.tryhisar.hisar.model.PasskeyAssertionRequest
import com.tryhisar.hisar.model.PasskeyRegistrationCredential
import com.tryhisar.hisar.model.PasskeyRegistrationRequest
import com.tryhisar.hisar.model.ThemeAppearanceName
import com.tryhisar.hisar.model.ThemeColorName
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultTheme
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.encodeToByteString

/** The AutoFill screens for passwords, one-time codes and passkeys, on websites and in Android apps. */
class AutoFillModelTests {
    private val backend = FakeBackend()
    private val allCompletedCredential = mutableListOf<LoginCredential>()
    private val allCompletedCode = mutableListOf<String>()
    private val allCopiedCode = mutableListOf<String>()
    private var cancelCount = 0
    private val allVerifiedApp = mutableListOf<AndroidApp>()
    private var allVouchingWebsite = emptyList<String>()
    private val allAssertion = mutableListOf<PasskeyAssertionCredential>()
    private val allRegistration = mutableListOf<PasskeyRegistrationCredential>()
    private var rejectedRegistrationCount = 0
    private var unlockCompletionCount = 0

    private val bankApp = AndroidApp("com.example.bank", "AB:".repeat(31) + "AB")
    private val bankAppTarget = FillTarget.App(bankApp, "Example Bank")

    private fun TestScope.createModel() = AutoFillModel(
        scope = backgroundScope,
        ioDispatcher = StandardTestDispatcher(testScheduler),
        createBackend = { backend },
        verifier = { app, _ ->
            allVerifiedApp += app
            allVouchingWebsite
        },
        completeRequest = { allCompletedCredential += it },
        completeOneTimeCodeRequest = { allCompletedCode += it },
        copyOneTimeCode = { allCopiedCode += it },
        cancelRequest = { cancelCount += 1 },
        passkeyCompletion = AutoFillModel.PasskeyCompletion(
            completeAssertion = { allAssertion += it },
            completeRegistration = { allRegistration += it },
            rejectExcludedRegistration = { rejectedRegistrationCount += 1 },
            completeUnlock = { unlockCompletionCount += 1 },
        ),
    )

    private fun websites(vararg allServiceIdentifier: String) = FillTarget.Websites(allServiceIdentifier.toList())

    @Test
    fun testSignedOutShowsMessage() = runTest {
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        assertEquals(AutoFillModel.Screen.SignedOut, model.screen.value)
    }

    @Test
    fun testFollowsTheAppTheme() = runTest {
        backend.theme = VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.GREEN)
        val model = createModel()
        assertEquals(VaultTheme.standard, model.theme.value)
        model.prepareCredentialList(websites("github.com"))
        assertEquals(VaultTheme(ThemeAppearanceName.LIGHT, ThemeColorName.GREEN), model.theme.value)
    }

    @Test
    fun testLockedThenUnlockListsMatchingLogins() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareCredentialList(websites("https://github.com/login", "github.com"))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertEquals("github.com", model.serviceName)
        val isUnlocked = model.unlock(FakeBackend.PASSWORD)
        assertTrue(isUnlocked)
        assertEquals(AutoFillModel.Screen.List, model.screen.value)
        assertEquals(listOf("GitHub"), model.allMatchingLogin.value.map { it.title }, "the same login matched twice is listed once")
    }

    @Test
    fun testWrongPasswordStaysLocked() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        val isUnlocked = model.unlock("nope")
        assertFalse(isUnlocked)
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertNotNull(model.errorMessage.value)
    }

    @Test
    fun testSelectingMatchingLoginCompletesWithPassword() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(websites("www.netflix.com"))
        assertEquals(AutoFillModel.Screen.List, model.screen.value)
        model.select(model.allMatchingLogin.value.first())
        assertEquals(listOf(LoginCredential("family@example.com", "netflix-password")), allCompletedCredential)
        assertTrue("getLoginForService" in backend.allCallName)
        assertEquals(emptyList(), allCopiedCode, "a login without a one-time password copies nothing")
    }

    @Test
    fun testSearchFindsOtherLoginsAndSelectsThem() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        model.updateSearchText("bank")
        model.loadAllOtherLogin()
        assertTrue(model.isOtherLoginLoaded.value)
        assertTrue(model.allVisibleMatchingLogin.isEmpty())
        assertEquals(listOf("Bank"), model.allVisibleOtherLogin.map { it.title })
        model.select(model.allVisibleOtherLogin[0])
        assertEquals(listOf(LoginCredential("alice@example.com", "bank-password")), allCompletedCredential)
    }

    @Test
    fun testSearchHidesMatchingLoginsFromOtherSection() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        model.updateSearchText("git")
        model.loadAllOtherLogin()
        assertEquals(listOf("GitHub"), model.allVisibleMatchingLogin.map { it.title })
        assertTrue(model.allVisibleOtherLogin.isEmpty())
    }

    @Test
    fun testSuggestionRequestUnlocksThenCompletes() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareInterfaceToProvideCredential("personal/github", websites("github.com"), LoginSource.Website("github.com"))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertTrue(allCompletedCredential.isEmpty())
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(listOf(LoginCredential("alice", "github-password", hasOneTimePassword = true)), allCompletedCredential)
        assertEquals(listOf(FakeBackend.oneTimeCode.code), allCopiedCode, "the code is on the clipboard for the site's next step")
    }

    @Test
    fun testFillingALoginWithACodeCopiesTheCodeFirst() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        model.select(model.allMatchingLogin.value[0])
        assertEquals(listOf(FakeBackend.oneTimeCode.code), allCopiedCode)
        assertEquals(listOf("github-password"), allCompletedCredential.map { it.password })
        assertTrue("getOneTimeCodeForService" in backend.allCallName)
    }

    @Test
    fun testOneTimeCodeListOffersOnlyLoginsWithACode() = runTest {
        backend.seedSignedIn()
        backend.allItemByVaultID["personal"]?.add(
            ItemDetail(id = "github-2", category = "login", title = "GitHub second", username = "bob", allURL = listOf("github.com"), password = "x"),
        )
        val model = createModel()
        model.prepareCredentialList(websites("github.com"), RequestKind.ONE_TIME_CODE)
        assertEquals(RequestKind.ONE_TIME_CODE, model.requestKind.value)
        assertEquals(2, model.allMatchingLogin.value.size)
        assertEquals(listOf("GitHub"), model.allVisibleMatchingLogin.map { it.title })
        model.select(model.allVisibleMatchingLogin[0])
        assertEquals(listOf(FakeBackend.oneTimeCode.code), allCompletedCode)
        assertTrue(allCompletedCredential.isEmpty(), "a one-time code request never hands out the password")
        assertFalse("getLoginForService" in backend.allCallName)

        model.prepareCredentialList(websites("www.netflix.com"), RequestKind.ONE_TIME_CODE)
        assertTrue(model.allVisibleMatchingLogin.isEmpty())
        model.updateSearchText("hub")
        model.loadAllOtherLogin()
        assertEquals(listOf("GitHub"), model.allVisibleOtherLogin.map { it.title })
    }

    @Test
    fun testOneTimeCodeSuggestionRequestUnlocksThenCompletes() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareInterfaceToProvideCredential("personal/github", websites("github.com"), LoginSource.Website("github.com"), RequestKind.ONE_TIME_CODE)
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(listOf(FakeBackend.oneTimeCode.code), allCompletedCode)
        assertTrue(allCompletedCredential.isEmpty())
    }

    @Test
    fun testOneTimeCodeWithoutInteractionNeedsAnUnlockedProcessAndAMatchingSite() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        val github = LoginSource.Website("github.com")
        assertNull(model.provideOneTimeCodeWithoutUserInteraction("personal/github", github))
        model.prepareCredentialList(websites("github.com"), RequestKind.ONE_TIME_CODE)
        assertEquals(FakeBackend.oneTimeCode.code, model.provideOneTimeCodeWithoutUserInteraction("personal/github", github))
        assertNull(model.provideOneTimeCodeWithoutUserInteraction("personal/github", LoginSource.Website("evil.example")))
        assertNull(model.provideOneTimeCodeWithoutUserInteraction("personal/bank", LoginSource.Website("bank.example.com")))
    }

    @Test
    fun testWithoutInteractionNeedsAnUnlockedProcess() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        val github = LoginSource.Website("github.com")
        assertNull(model.provideCredentialWithoutUserInteraction("personal/github", github), "no backend is prepared yet")
        model.prepareCredentialList(websites("github.com"))
        model.unlock(FakeBackend.PASSWORD)
        assertEquals("github-password", model.provideCredentialWithoutUserInteraction("personal/github", github)?.password)
        assertEquals(listOf(FakeBackend.oneTimeCode.code), allCopiedCode)
        assertNull(model.provideCredentialWithoutUserInteraction("personal/github", LoginSource.Website("evil.example")))
    }

    @Test
    fun testSuggestionAfterAutoLockAsksForTheUnlock() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        val github = LoginSource.Website("github.com")
        model.prepare()
        assertEquals("github-password", model.provideCredentialWithoutUserInteraction("personal/github", github)?.password)
        backend.lock()
        assertNull(model.provideCredentialWithoutUserInteraction("personal/github", github), "the backend answers locked once auto-lock is due")
        model.prepareInterfaceToProvideCredential("personal/github", websites("github.com"), github)
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertTrue(allCompletedCredential.isEmpty())
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(listOf("github-password"), allCompletedCredential.map { it.password })
    }

    @Test
    fun testCancel() = runTest {
        val model = createModel()
        model.cancel()
        assertEquals(1, cancelCount)
    }

    @Test
    fun testBiometricUnlock() = runTest {
        backend.seedSignedIn()
        backend.enableBiometricUnlock(FakeBackend.PASSWORD)
        backend.lock()
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        assertTrue(model.status.value.biometricReady)
        model.unlockWithBiometric()
        assertEquals(AutoFillModel.Screen.List, model.screen.value)
        assertEquals(1, model.allMatchingLogin.value.size)
    }

    @Test
    fun testAppListsItsLinkedLoginsThenTheLoginsOfWebsitesThatVouchForIt() = runTest {
        backend.seedSignedIn()
        linkBankLogin(bankApp)
        allVouchingWebsite = listOf("https://github.com")
        val model = createModel()
        model.prepareCredentialList(bankAppTarget)
        assertEquals("Example Bank", model.serviceName)
        assertEquals(listOf(bankApp), allVerifiedApp)
        assertEquals(listOf("Bank", "GitHub"), model.allMatchingLogin.value.map { it.title })

        model.select(model.allMatchingLogin.value[0])
        assertTrue("getLoginForAndroidApp" in backend.allCallName, "a linked login is checked against the app again")
        model.select(model.allMatchingLogin.value[1])
        assertTrue("getLoginForService" in backend.allCallName, "a vouching website's login is checked against the website again")
        assertEquals(listOf("bank-password", "github-password"), allCompletedCredential.map { it.password })
    }

    @Test
    fun testAppWithAnotherCertificateGetsNoLinkedLogin() = runTest {
        backend.seedSignedIn()
        linkBankLogin(bankApp)
        val model = createModel()
        model.prepareCredentialList(FillTarget.App(bankApp.copy(certificateSHA256 = "CD:".repeat(31) + "CD"), "Example Bank"))
        assertTrue(model.allMatchingLogin.value.isEmpty())
    }

    @Test
    fun testLoginFoundBySearchForAnAppIsLinkedWhenTheUserAsks() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(bankAppTarget)
        assertTrue(model.allMatchingLogin.value.isEmpty())
        model.updateSearchText("bank")
        model.loadAllOtherLogin()
        model.select(model.allVisibleOtherLogin.single())
        assertEquals("Bank", model.loginToLink.value?.title)
        assertTrue(allCompletedCredential.isEmpty(), "nothing is filled before the user answers")

        model.fillLoginToLink(isLinked = true)
        assertNull(model.loginToLink.value)
        assertEquals(listOf(ItemAndroidApp(bankApp.packageName, bankApp.certificateSHA256)), backend.getItem("personal", "bank").allAndroidApp)
        assertEquals(listOf("bank-password"), allCompletedCredential.map { it.password })

        model.prepareCredentialList(bankAppTarget)
        assertEquals(listOf("Bank"), model.allMatchingLogin.value.map { it.title }, "the next fill offers the linked login")
    }

    @Test
    fun testLoginFoundBySearchForAnAppFillsOnceWithoutALink() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(bankAppTarget)
        model.updateSearchText("bank")
        model.loadAllOtherLogin()
        model.select(model.allVisibleOtherLogin.single())
        model.fillLoginToLink(isLinked = false)
        assertEquals(listOf("bank-password"), allCompletedCredential.map { it.password })
        assertFalse("addAndroidApp" in backend.allCallName)
        assertTrue(backend.getItem("personal", "bank").allAndroidApp.isEmpty())
    }

    @Test
    fun testUnlockFinishesAfterTheLockScreenLeaves() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareCredentialList(websites("github.com"))
        val lockScreen = launch { model.unlock(FakeBackend.PASSWORD) }
        launch {
            model.screen.first { it == AutoFillModel.Screen.List }
            lockScreen.cancel()
        }
        lockScreen.join()
        backgroundScope.coroutineContext.job.children.forEach { it.join() }
        assertEquals(listOf("GitHub"), model.allMatchingLogin.value.map { it.title }, "the list loads though the screen that unlocked is gone")
        assertNull(model.errorMessage.value)
    }

    @Test
    fun testRecordIdentifierRoundTrip() {
        val recordIdentifier = LoginMatch.buildRecordIdentifier("vault-1", "item/with/slash")
        assertEquals("vault-1" to "item/with/slash", LoginMatch.parseRecordIdentifier(recordIdentifier))
        assertNull(LoginMatch.parseRecordIdentifier("nothing"))
        assertNull(LoginMatch.parseRecordIdentifier("/item"))
    }

    private val clientDataHash = ByteString(ByteArray(32) { it.toByte() })
    private val noWebsite = FillTarget.Websites(emptyList())

    @Test
    fun testPasskeyListShowsPasskeysForTheRelyingPartyAndSigns() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        val request = PasskeyAssertionRequest("github.com", clientDataHash)
        model.prepareCredentialList(websites("https://github.com"), passkeyRequest = request)
        assertEquals(listOf("GitHub"), model.allVisibleMatchingPasskey.map { it.title })
        assertEquals(listOf("GitHub"), model.allMatchingLogin.value.map { it.title }, "passwords are still offered")
        model.selectPasskey(model.allVisibleMatchingPasskey[0])
        assertEquals(1, allAssertion.size)
        assertEquals("github.com", allAssertion.first().relyingPartyID)
        assertEquals(clientDataHash, allAssertion.first().clientDataHash)
        assertEquals(ByteString(1, 1, 1), allAssertion.first().credentialID)
        assertEquals("alice-handle".encodeToByteString(), allAssertion.first().userHandle)
        assertEquals(ByteString("signature over ".encodeToByteArray() + clientDataHash.toByteArray()), allAssertion.first().signature)
        assertTrue(allCompletedCredential.isEmpty())
    }

    @Test
    fun testPasskeyListAppliesAllowedCredentialsAndOtherRelyingParties() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareCredentialList(websites("https://github.com"), passkeyRequest = PasskeyAssertionRequest("github.com", clientDataHash, listOf(ByteString(9))))
        assertTrue(model.allVisibleMatchingPasskey.isEmpty())
        model.prepareCredentialList(websites("https://gist.github.com"), passkeyRequest = PasskeyAssertionRequest("gist.github.com", clientDataHash))
        assertTrue(model.allVisibleMatchingPasskey.isEmpty(), "only passkeys whose RP ID equals the request's")
        model.prepareCredentialList(websites("github.com"))
        assertNull(model.passkeyAssertionRequest.value)
        assertTrue(model.allMatchingPasskey.value.isEmpty(), "a password-only request lists no passkeys")
    }

    @Test
    fun testLockedPasskeyListUnlocksThenLists() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareCredentialList(noWebsite, passkeyRequest = PasskeyAssertionRequest("github.com", clientDataHash))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertEquals("github.com", model.serviceName)
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(AutoFillModel.Screen.List, model.screen.value)
        assertEquals(1, model.allVisibleMatchingPasskey.size)
        model.updateSearchText("nobody")
        assertTrue(model.allVisibleMatchingPasskey.isEmpty())
    }

    @Test
    fun testChosenPasskeyUnlocksThenSigns() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        val request = PasskeyAssertionRequest("github.com", clientDataHash)
        model.prepareInterfaceToProvidePasskey(request, ByteString(1, 1, 1))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertTrue(allAssertion.isEmpty())
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(listOf(ByteString(1, 1, 1)), allAssertion.map { it.credentialID })
    }

    @Test
    fun testChosenPasskeyOfAnotherSiteFallsBackToTheList() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareInterfaceToProvidePasskey(PasskeyAssertionRequest("evil.example", clientDataHash), ByteString(1, 1, 1))
        assertTrue(allAssertion.isEmpty())
        assertEquals(AutoFillModel.Screen.List, model.screen.value)
        assertTrue(model.allMatchingPasskey.value.isEmpty())
    }

    @Test
    fun testPasskeyWithoutInteractionNeedsAnUnlockedProcess() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        val request = PasskeyAssertionRequest("github.com", clientDataHash)
        assertNull(model.providePasskeyWithoutUserInteraction(request, ByteString(1, 1, 1)), "no backend is prepared yet")
        model.prepareCredentialList(websites("github.com"))
        assertEquals("alice-handle".encodeToByteString(), model.providePasskeyWithoutUserInteraction(request, ByteString(1, 1, 1))?.userHandle)
        backend.lock()
        assertNull(model.providePasskeyWithoutUserInteraction(request, ByteString(1, 1, 1)))
    }

    @Test
    fun testPasskeyRegistrationPreselectsTheMatchingLoginAndSaves() = runTest {
        backend.seedSignedIn()
        backend.allItemByVaultID["personal"]?.add(ItemDetail(id = "github-bob", category = "login", title = "GitHub Bob", username = "bob", allURL = listOf("github.com")))
        val model = createModel()
        val request = PasskeyRegistrationRequest("github.com", "Alice", "alice-handle".encodeToByteString(), clientDataHash, listOf(-8, -7))
        model.prepareInterfaceForPasskeyRegistration(request)
        assertEquals(AutoFillModel.Screen.PasskeyRegistration, model.screen.value)
        assertEquals(listOf("GitHub", "GitHub Bob"), model.passkeyRegistrationOptions.value?.allLogin?.map { it.title })
        assertEquals("personal/github", model.selectedRegistrationLoginID.value, "the login whose username equals the user name")
        model.savePasskey()
        assertEquals(1, allRegistration.size)
        assertEquals("github.com", allRegistration.first().relyingPartyID)
        assertEquals(clientDataHash, allRegistration.first().clientDataHash)
        assertEquals("attestation for github.com".encodeToByteString(), allRegistration.first().attestationObject)
        val savedItem = backend.getItem("personal", "github")
        assertEquals(1, savedItem.allPasskey.size, "the passkey with the same user handle is replaced")
        assertEquals(allRegistration.first().credentialID, Base64URL.decode(savedItem.allPasskey.first().credentialID))
    }

    @Test
    fun testPasskeyRegistrationToANewLogin() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("shop.example", "buyer", ByteString(5), clientDataHash))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertEquals("shop.example", model.serviceName)
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(AutoFillModel.Screen.PasskeyRegistration, model.screen.value)
        assertEquals(emptyList(), model.passkeyRegistrationOptions.value?.allLogin)
        assertNull(model.selectedRegistrationLoginID.value, "New login is the only choice")
        model.savePasskey()
        assertEquals(1, allRegistration.size)
        assertTrue(backend.allItemByVaultID["personal"].orEmpty().any { it.title == "shop.example" && it.allPasskey.size == 1 })
    }

    @Test
    fun testExcludedPasskeyRegistrationIsRejected() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareInterfaceForPasskeyRegistration(
            PasskeyRegistrationRequest("github.com", "alice", ByteString(5), clientDataHash, allExcludedCredentialID = listOf(ByteString(1, 1, 1))),
        )
        assertEquals(true, model.passkeyRegistrationOptions.value?.isExcluded)
        model.savePasskey()
        assertTrue(allRegistration.isEmpty())
        assertFalse("createPasskey" in backend.allCallName)
        model.rejectExcludedPasskeyRegistration()
        assertEquals(1, rejectedRegistrationCount)
    }

    @Test
    fun testPasskeyRegistrationWithoutES256IsCancelled() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("github.com", "alice", ByteString(5), clientDataHash, listOf(-8, -257)))
        assertEquals(1, cancelCount)
        assertFalse("preparePasskeyRegistration" in backend.allCallName)
    }

    @Test
    fun testPasskeyRegistrationErrorIsShown() = runTest {
        backend.seedSignedIn()
        val model = createModel()
        model.prepareInterfaceForPasskeyRegistration(PasskeyRegistrationRequest("github.com", "alice", ByteString(5), ByteString(1)))
        model.savePasskey()
        assertTrue(allRegistration.isEmpty())
        assertEquals("the request is not valid", model.errorMessage.value)
        assertEquals(AutoFillModel.Screen.PasskeyRegistration, model.screen.value)
    }

    @Test
    fun testUnlockActionUnlocksThenHandsBackWithoutAList() = runTest {
        backend.seedSignedIn(state = VaultStatus.STATE_LOCKED)
        val model = createModel()
        model.prepareUnlock(noWebsite, PasskeyAssertionRequest("github.com", clientDataHash))
        assertEquals(AutoFillModel.Screen.Locked, model.screen.value)
        assertEquals(0, unlockCompletionCount)
        model.unlock(FakeBackend.PASSWORD)
        assertEquals(1, unlockCompletionCount)
        assertEquals(AutoFillModel.Screen.Starting, model.screen.value, "the sheet lists the credentials, not Hisar")
        assertFalse("listAllPasskeyForRelyingParty" in backend.allCallName)
        model.prepareUnlock(noWebsite, null)
        assertEquals(2, unlockCompletionCount, "an unlocked Hisar hands back at once")
    }

    private fun linkBankLogin(app: AndroidApp) {
        backend.addAndroidApp("personal", "bank", app.packageName, app.certificateSHA256)
        backend.allCallName.clear()
    }
}
