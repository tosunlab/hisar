package com.tryhisar.hisar.autofill

import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.LoginCredential
import com.tryhisar.hisar.model.LoginMatch
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

/** The AutoFill screens for passwords and one-time codes, on websites and in Android apps. */
class AutoFillModelTests {
    private val backend = FakeBackend()
    private val allCompletedCredential = mutableListOf<LoginCredential>()
    private val allCompletedCode = mutableListOf<String>()
    private val allCopiedCode = mutableListOf<String>()
    private var cancelCount = 0
    private val allVerifiedApp = mutableListOf<AndroidApp>()
    private var allVouchingWebsite = emptyList<String>()

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

    private fun linkBankLogin(app: AndroidApp) {
        backend.addAndroidApp("personal", "bank", app.packageName, app.certificateSHA256)
        backend.allCallName.clear()
    }
}
