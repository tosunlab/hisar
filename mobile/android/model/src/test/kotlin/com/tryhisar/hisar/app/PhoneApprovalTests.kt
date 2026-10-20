package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.PhoneApprovalIcon
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.model.parseTimestamp
import java.io.ByteArrayInputStream
import java.time.Instant
import javax.imageio.ImageIO
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest

/** Answering the Mac's requests: what a request says, how the app finds requests, and what each answer does. */
class PhoneApprovalTests {
    private val currentDate: Instant = Instant.ofEpochSecond(1_800_000_000)

    private fun createApproval(
        requestID: String = "0123456789abcdef0123456789abcdef",
        secondsLeft: Long = 45,
        kind: String = PhoneApproval.KIND_SSH,
        isForwarded: Boolean = false,
    ) = PhoneApproval(
        requestID = requestID, requesterDeviceName = "Alper's MacBook Pro", expiresAt = currentDate.plusSeconds(secondsLeft).toString(),
        kind = kind, appName = "Terminal", purpose = "push to git.tryhisar.com", host = "git.tryhisar.com:22", commandLine = "git push origin main",
        repositoryPath = "~/src/core", keyName = "hisar gitlab", isForwarded = isForwarded, forwardedFromHost = if (isForwarded) "build.example.com" else "",
    )

    private fun createRememberingApproval(requestID: String = "0123456789abcdef0123456789abcdef", isRememberedByDefault: Boolean = true) =
        createApproval(requestID).copy(rememberLabel = "Also allows Terminal for 1 hour", isRememberedByDefault = isRememberedByDefault)

    // A launch with the app in the foreground: unlocking lists the open requests at once.
    private suspend fun TestScope.createUnlockedModel(harness: AppModelHarness, allApproval: List<PhoneApproval>): AppModel {
        harness.currentDate = currentDate
        harness.backend.seedSignedIn()
        harness.backend.allPendingApproval = allApproval.toMutableList()
        val model = harness.startModel()
        model.approvals.refreshPendingApproval()
        runCurrent()
        return model
    }

    private suspend fun TestScope.createReviewingModel(harness: AppModelHarness, allApproval: List<PhoneApproval>): AppModel {
        val model = createUnlockedModel(harness, allApproval)
        model.approvals.allPendingApproval.value.firstOrNull()?.let(model.approvals::reviewApproval)
        return model
    }

    private fun decode(json: String): List<PhoneApproval> = backendJson.decodeFromString(json)

    @Test
    fun testDecodesTheBackendsRequest() {
        val allApproval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"cli","appName":"hisar in Terminal","purpose":"use Hisar","host":"","commandLine":"/Applications/Hisar.app/Contents/Helpers/hisar","repositoryPath":"",
              "keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"ttys003"}]
            """,
        )
        assertEquals(1, allApproval.size)
        val approval = allApproval[0]
        assertEquals("Approve on MacBook Pro?", approval.title)
        assertEquals("hisar in Terminal wants to use Hisar", approval.summary)
        assertEquals("MacBook Pro is asking to use Hisar from the command line", approval.bannerTitle)
        assertEquals(PhoneApprovalIcon.TERMINAL, approval.icon)
        assertEquals(listOf("App", "Program", "Terminal"), approval.allDetailRow.map { it.label })
        assertEquals("/dev/ttys003", approval.allDetailRow.last().value)
        assertNotNull(approval.expiryDate)
        assertFalse(approval.isExpired(parseTimestamp("2027-01-15T08:00:59Z")!!))
        assertTrue(approval.isExpired(parseTimestamp("2027-01-15T08:01:01Z")!!))
    }

    @Test
    fun testDescribesARunRequestWithItsSecrets() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"cli","appName":"hisar in Terminal","purpose":"run a command with 5 secrets","commandLine":"npm test","terminalName":"ttys003",
              "secrets":["Personal › GitHub › password","Work › AWS › access key","and 3 more"]}]
            """,
        ).first()
        assertEquals("hisar in Terminal wants to run a command with 5 secrets", approval.summary)
        assertEquals("MacBook Pro is asking to use Hisar from the command line", approval.bannerTitle)
        assertEquals(listOf("App", "Command", "Secrets", "Terminal"), approval.allDetailRow.map { it.label })
        assertEquals("Personal › GitHub › password\nWork › AWS › access key\nand 3 more", approval.allDetailRow[2].value)
    }

    @Test
    fun testCodingAgentRequestShowsItsProject() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"cli","appName":"omp in Tern (via hisar)","purpose":"use 1 secret","commandLine":"npm run db:migrate","repositoryPath":"~/src/acme-web/packages/db",
              "projectPath":"~/src/acme-web","secrets":["Agents › Stripe › secret key"]}]
            """,
        ).first()
        assertEquals(listOf("App", "Project", "Command", "Repository", "Secrets"), approval.allDetailRow.map { it.label })
        assertEquals(PhoneApproval.DetailRow("Project", "~/src/acme-web", true), approval.allDetailRow[1])
        assertEquals("", createApproval().projectPath, "a request from an older Mac has no project")
    }

    @Test
    fun testCodingAgentRequestOmitsTheRepositoryThatIsItsProject() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"cli","appName":"omp in Tern (via hisar)","purpose":"use 1 secret","commandLine":"npm run db:migrate","repositoryPath":"~/src/acme-web/",
              "projectPath":"~/src/acme-web","secrets":["Agents › Stripe › secret key"]}]
            """,
        ).first()
        assertEquals(listOf("App", "Project", "Command", "Secrets"), approval.allDetailRow.map { it.label })
        assertEquals("~/src/acme-web", approval.allDetailRow[1].value)
    }

    @Test
    fun testLauncherRequestNamesTheAppAndHowLongItLasts() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"launcher","appName":"Raycast","purpose":"search Hisar and copy or paste passwords and one-time codes","host":"","commandLine":"",
              "repositoryPath":"","keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"",
              "appIcon":"$ONE_PIXEL_PNG"}]
            """,
        ).first()
        assertEquals("Approve on MacBook Pro?", approval.title)
        assertEquals("Raycast wants to search Hisar and copy or paste passwords and one-time codes", approval.summary)
        assertEquals("MacBook Pro is asking to let Raycast use Hisar", approval.bannerTitle)
        assertEquals(PhoneApprovalIcon.LAUNCHER, approval.icon)
        assertEquals(listOf("App", "Lasts"), approval.allDetailRow.map { it.label })
        assertEquals(listOf("Raycast", "Until Hisar locks on MacBook Pro"), approval.allDetailRow.map { it.value })
        assertEquals("Until Hisar locks on MacBook Pro", approval.detailLine, "the card says how long approving lasts, as there is no command")
        assertEquals("Approving lets Raycast use Hisar until it locks on your Mac; it does not unlock Hisar there.", approval.approvalNote)
        assertFalse(approval.isRememberable)
    }

    @Test
    fun testLauncherShareLinkRequestIsApprovedOnce() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","createdAt":"2027-01-15T08:00:00.123Z","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"launcher","appName":"Raycast","purpose":"create a share link for “GitHub”","host":"","commandLine":"Expires in 7 days · 1 view",
              "repositoryPath":"","keyName":"","isForwarded":false,"forwardedFromHost":"","terminalName":"","secrets":["username","password"],
              "appIcon":"$ONE_PIXEL_PNG",
              "appWarning":"Anyone with the link can read these until it expires or its views run out."}]
            """,
        ).first()
        assertEquals("Raycast wants to create a share link for “GitHub”", approval.summary)
        assertEquals(listOf("App", "Secrets", "Link", "Warning"), approval.allDetailRow.map { it.label }, "a share link lasts for this one request")
        assertEquals(
            listOf("Raycast", "username\npassword", "Expires in 7 days · 1 view", "Anyone with the link can read these until it expires or its views run out."),
            approval.allDetailRow.map { it.value },
        )
        assertEquals("Approving lets this one request continue; it does not unlock Hisar on your Mac.", approval.approvalNote)
        assertEquals("username, password", approval.detailLine, "the card names what the link shares")
        assertFalse(approval.isRememberable)
    }

    @Test
    fun testDecodesTheAppIconAndSignatureWarning() {
        val approval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"ssh","appName":"iTerm2","purpose":"sign a commit","commandLine":"“Fix the login redirect”",
              "appIcon":"$ONE_PIXEL_PNG",
              "appWarning":"iTerm2 is not signed, so macOS cannot confirm who made it."}]
            """,
        ).first()
        assertEquals("iTerm2 wants to sign a commit", approval.summary)
        val appIcon = assertNotNull(approval.appIcon)
        assertNotNull(ImageIO.read(ByteArrayInputStream(appIcon.toByteArray())), "the icon is an image")
        assertEquals(listOf("App", "Warning", "Command"), approval.allDetailRow.map { it.label })
        assertNull(createApproval().appIcon)
    }

    @Test
    fun testDescribesRequestsForTheBanner() {
        val approval = createApproval(secondsLeft = 115, isForwarded = true)
        assertEquals("build.example.com wants to push to git.tryhisar.com", approval.summary)
        assertEquals("Alper's MacBook Pro is asking to use “hisar gitlab”", approval.bannerTitle)
        assertEquals("Expires in 1:55", approval.formatTimeLeft(currentDate))
        assertEquals("Expires in 1 second", approval.formatTimeLeft(currentDate.plusSeconds(114)))
        assertEquals("Expires in 35 seconds", approval.formatTimeLeft(currentDate.plusSeconds(80)))
        assertEquals(listOf("App", "Command", "Repository", "Server", "Key", "Forwarded from"), approval.allDetailRow.map { it.label })
        assertEquals(listOf("App", "Command", "Repository", "Server", "Key"), createApproval().allDetailRow.map { it.label })
    }

    @Test
    fun testUnlockListsWaitingRequestsWithoutOpeningTheSheet() = runTest {
        val model = createUnlockedModel(
            AppModelHarness(this),
            listOf(createApproval("expired", secondsLeft = -1), createApproval("first"), createApproval("second")),
        )
        val approvals = model.approvals
        assertEquals(listOf("first", "second"), approvals.allPendingApproval.value.map { it.requestID })
        assertNull(approvals.presentedApproval.value)
        approvals.reviewApproval(approvals.allPendingApproval.value[1])
        assertEquals("second", approvals.presentedApproval.value?.requestID)
        approvals.closeApprovalSheet()
        assertNull(approvals.presentedApproval.value)
        assertEquals(2, approvals.allPendingApproval.value.size, "closing the sheet keeps the request in the list")
    }

    @Test
    fun testARequestArrivingInTheForegroundOpensTheSheet() = runTest {
        val harness = AppModelHarness(this)
        val model = createUnlockedModel(harness, listOf(createApproval("listed-at-unlock")))
        assertNull(model.approvals.presentedApproval.value)
        harness.backend.allPendingApproval.add(createApproval("new"))
        harness.events.emit(BackendEvent.Approval(PhoneApproval.EVENT_REQUESTED, "new"))
        runCurrent()
        assertEquals("new", model.approvals.presentedApproval.value?.requestID)
        assertEquals(2, model.approvals.allPendingApproval.value.size)
    }

    @Test
    fun testDecodesTheRememberSwitch() {
        val allApproval = decode(
            """
            [{"requestId":"0123456789abcdef0123456789abcdef","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"ssh","appName":"iTerm2","purpose":"sign a commit","repositoryPath":"~/src/core",
              "rememberLabel":"Also allows iTerm2 to sign commits in ~/src/core for 10 minutes"},
             {"requestId":"fedcba9876543210fedcba9876543210","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com","rememberLabel":"Also allows Terminal for 1 hour","isRememberedByDefault":true},
             {"requestId":"00112233445566778899aabbccddeeff","requesterDeviceName":"MacBook Pro","expiresAt":"2027-01-15T08:01:00.123Z",
              "kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com"}]
            """,
        )
        assertEquals(listOf(true, true, false), allApproval.map { it.isRememberable })
        assertEquals("Also allows iTerm2 to sign commits in ~/src/core for 10 minutes", allApproval[0].rememberLabel)
        assertFalse(allApproval[0].resolveRememberChoice(null), "commit signing starts off")
        assertTrue(allApproval[0].resolveRememberChoice(true))
        assertTrue(allApproval[1].resolveRememberChoice(null), "the switch starts as the Mac's checkbox does")
        assertFalse(allApproval[1].resolveRememberChoice(false))
        assertFalse(allApproval[2].resolveRememberChoice(true), "a request without the switch is never remembered")
    }

    @Test
    fun testCardApprovesWithTheRequestsDefault() = runTest {
        val harness = AppModelHarness(this)
        val model = createUnlockedModel(
            harness,
            listOf(createRememberingApproval("ssh"), createRememberingApproval("signing", isRememberedByDefault = false), createApproval("once")),
        )
        repeat(3) { model.approvals.approve(model.approvals.allPendingApproval.value[0]) }
        assertEquals(listOf("approve remembered ssh", "approve signing", "approve once"), harness.backend.allApprovalAnswer)
    }

    @Test
    fun testCardFooterNamesWhatApproveSends() {
        assertEquals(
            "Also allows Terminal for 1 hour. Tap the request to approve it once. It never unlocks Hisar on your Mac.",
            createRememberingApproval("ssh").cardApprovalNote,
        )
        assertEquals(
            "Approving answers this one request. It never unlocks Hisar on your Mac.",
            createRememberingApproval("signing", isRememberedByDefault = false).cardApprovalNote,
        )
        assertEquals("Approving answers this one request. It never unlocks Hisar on your Mac.", createApproval("once").cardApprovalNote)
    }

    @Test
    fun testSheetSwitchIsSentWithTheApproval() = runTest {
        val harness = AppModelHarness(this)
        val model = createReviewingModel(harness, listOf(createRememberingApproval("remembered"), createRememberingApproval("once")))
        model.approvals.approvePendingApproval(isRemembered = true)
        model.approvals.reviewApproval(model.approvals.allPendingApproval.value[0])
        model.approvals.approvePendingApproval(isRemembered = false)
        assertEquals(listOf("approve remembered remembered", "approve once"), harness.backend.allApprovalAnswer)
    }

    @Test
    fun testSheetSwitchIsSentWithThePasswordApproval() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricAvailable = false
        val model = createReviewingModel(harness, listOf(createRememberingApproval()))
        model.approvals.approvePendingApproval(isRemembered = true)
        assertTrue(model.approvals.isApprovalPasswordRequired.value)
        model.approvals.approvePendingApproval(password = FakeBackend.PASSWORD, isRemembered = true)
        assertEquals(listOf("approve remembered 0123456789abcdef0123456789abcdef"), harness.backend.allApprovalAnswer)
    }

    @Test
    fun testOldRequestsApproveOnce() = runTest {
        val harness = AppModelHarness(this)
        val model = createUnlockedModel(harness, listOf(createApproval("old")))
        model.approvals.reviewApproval(model.approvals.allPendingApproval.value[0])
        model.approvals.approvePendingApproval(isRemembered = true)
        assertEquals(listOf("approve old"), harness.backend.allApprovalAnswer, "a request without the switch is never remembered")
    }

    @Test
    fun testApproveWithBiometrics() = runTest {
        val harness = AppModelHarness(this)
        val model = createReviewingModel(harness, listOf(createApproval()))
        model.approvals.approvePendingApproval(isRemembered = false)
        assertEquals(listOf("approve 0123456789abcdef0123456789abcdef"), harness.backend.allApprovalAnswer)
        assertNull(model.approvals.presentedApproval.value)
        assertTrue(model.approvals.allPendingApproval.value.isEmpty())
        assertEquals("Approved on your Mac", model.notice.value)
        assertNull(model.approvals.approvalErrorMessage.value)
    }

    @Test
    fun testDenyNeedsNoBiometrics() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricSucceeding = false
        val model = createReviewingModel(harness, listOf(createApproval()))
        model.approvals.denyPendingApproval()
        assertEquals(listOf("deny 0123456789abcdef0123456789abcdef"), harness.backend.allApprovalAnswer)
        assertNull(model.approvals.presentedApproval.value)
        assertEquals("Denied on your Mac", model.notice.value)
    }

    @Test
    fun testCancelledBiometricsKeepTheRequest() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricSucceeding = false
        val model = createReviewingModel(harness, listOf(createApproval()))
        model.approvals.approvePendingApproval(isRemembered = false)
        assertEquals(emptyList<String>(), harness.backend.allApprovalAnswer)
        assertNotNull(model.approvals.presentedApproval.value)
        assertNull(model.approvals.approvalErrorMessage.value)
        assertFalse(model.approvals.isApprovalPasswordRequired.value)
    }

    @Test
    fun testFallsBackToTheMasterPassword() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.isBiometricAvailable = false
        val model = createReviewingModel(harness, listOf(createApproval()))
        val approvals = model.approvals
        approvals.approvePendingApproval(isRemembered = false)
        assertTrue(approvals.isApprovalPasswordRequired.value)
        assertNull(approvals.approvalErrorMessage.value)
        approvals.approvePendingApproval(password = "wrong", isRemembered = false)
        assertEquals("Incorrect master password.", approvals.approvalErrorMessage.value)
        assertNotNull(approvals.presentedApproval.value)
        approvals.approvePendingApproval(password = FakeBackend.PASSWORD, isRemembered = false)
        assertEquals(listOf("approve 0123456789abcdef0123456789abcdef"), harness.backend.allApprovalAnswer)
        assertNull(approvals.presentedApproval.value)
        assertFalse(approvals.isApprovalPasswordRequired.value)
    }

    @Test
    fun testRequestThatIsGoneCloses() = runTest {
        val harness = AppModelHarness(this)
        val model = createReviewingModel(harness, listOf(createApproval()))
        harness.backend.allPendingApproval = mutableListOf()
        model.approvals.approvePendingApproval(isRemembered = false)
        assertNull(model.approvals.presentedApproval.value)
        assertTrue(model.approvals.allPendingApproval.value.isEmpty())
        assertEquals("The request is no longer waiting", model.notice.value)
    }

    @Test
    fun testAnsweredWithdrawnAndExpiredRequestsLeaveTheList() = runTest {
        val harness = AppModelHarness(this)
        val model = createReviewingModel(
            harness,
            listOf(createApproval("first"), createApproval("second", secondsLeft = 90), createApproval("third", secondsLeft = 20)),
        )
        val approvals = model.approvals
        assertEquals("first", approvals.presentedApproval.value?.requestID)
        approvals.handleApprovalEvent(PhoneApproval.EVENT_RESOLVED, "first")
        assertNull(approvals.presentedApproval.value, "the Mac withdrew the request under review")
        assertEquals(listOf("second", "third"), approvals.allPendingApproval.value.map { it.requestID })
        approvals.handleApprovalEvent(PhoneApproval.EVENT_ANSWERED, "second")
        assertEquals(listOf("third"), approvals.allPendingApproval.value.map { it.requestID })
        harness.currentDate = harness.currentDate.plusSeconds(21)
        approvals.removeAllExpiredApproval()
        assertTrue(approvals.allPendingApproval.value.isEmpty())
        approvals.refreshPendingApproval()
        assertTrue(approvals.allPendingApproval.value.isEmpty(), "finished requests are not shown again")
    }

    @Test
    fun testNothingWhileLocked() = runTest {
        val harness = AppModelHarness(this)
        val model = createReviewingModel(harness, listOf(createApproval()))
        assertNotNull(model.approvals.presentedApproval.value)
        model.lock()
        assertNull(model.approvals.presentedApproval.value)
        assertTrue(model.approvals.allPendingApproval.value.isEmpty())
        model.approvals.refreshPendingApproval()
        assertTrue(model.approvals.allPendingApproval.value.isEmpty())
    }

    @Test
    fun testClassifiesApprovalErrors() {
        assertEquals(ApprovalsModel.ErrorKind.GONE, ApprovalsModel.classifyApprovalError("This request is no longer waiting for an answer"))
        assertEquals(ApprovalsModel.ErrorKind.CANCELLED, ApprovalsModel.classifyApprovalError("The biometric check was cancelled."))
        assertEquals(ApprovalsModel.ErrorKind.NEEDS_PASSWORD, ApprovalsModel.classifyApprovalError("Enter your master password."))
        assertEquals(ApprovalsModel.ErrorKind.NEEDS_PASSWORD, ApprovalsModel.classifyApprovalError("Biometrics are locked. Unlock with your master password."))
        assertEquals(ApprovalsModel.ErrorKind.NEEDS_PASSWORD, ApprovalsModel.classifyApprovalError("Biometrics are not available; approve with your master password"))
        assertEquals(ApprovalsModel.ErrorKind.OTHER, ApprovalsModel.classifyApprovalError("Incorrect master password."))
        assertEquals(ApprovalsModel.ErrorKind.OTHER, ApprovalsModel.classifyApprovalError("Cannot reach the Hisar server"))
    }

    private companion object {
        const val ONE_PIXEL_PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
    }
}
