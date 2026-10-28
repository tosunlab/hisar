package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Base64URL
import com.tryhisar.hisar.model.FakeBackend
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlinx.io.bytestring.encodeToByteString

/**
 * The approval push's notification: the signed preview, which Go opens through HisarKit, names the Mac, the app and the
 * request; anything missing, expired or wrongly signed keeps the kind's own sentence.
 */
class ApprovalPushTests {
    private fun message(previewJson: String): String {
        val preview = Base64URL.encode(previewJson.encodeToByteString())
        return """{"kind":"approval","approval":{"preview":"$preview","signature":"c2ln"}}"""
    }

    @Test
    fun testAnApprovalPushNamesTheMacTheAppAndTheRequest() {
        val backend = FakeBackend()

        val ssh = backend.openApprovalPush(
            message(
                """{"requestId":"req-1","requesterDeviceName":"MacBook Pro","expiresAt":"2026-10-27T15:04:05Z","kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com","keyName":"GitHub"}""",
            ),
        )!!
        assertEquals("MacBook Pro is asking to use “GitHub”", ssh.bannerTitle)
        assertEquals("Terminal wants to push to git.tryhisar.com", ssh.summary)

        val cli = backend.openApprovalPush(
            message(
                """{"requestId":"req-2","requesterDeviceName":"MacBook Pro","expiresAt":"2026-10-27T15:04:05Z","kind":"cli","appName":"Terminal","purpose":"read “GitHub token”"}""",
            ),
        )!!
        assertEquals("MacBook Pro is asking to use Hisar from the command line", cli.bannerTitle)
        assertEquals("Terminal wants to read “GitHub token”", cli.summary)

        val launcher = backend.openApprovalPush(
            message(
                """{"requestId":"req-3","requesterDeviceName":"MacBook Pro","expiresAt":"2026-10-27T15:04:05Z","kind":"launcher","appName":"Raycast","purpose":"use Hisar"}""",
            ),
        )!!
        assertEquals("MacBook Pro is asking to let Raycast use Hisar", launcher.bannerTitle)
        assertEquals("Raycast wants to use Hisar", launcher.summary)

        val forwarded = backend.openApprovalPush(
            message(
                """{"requestId":"req-4","requesterDeviceName":"MacBook Pro","expiresAt":"2026-10-27T15:04:05Z","kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com","keyName":"GitHub","isForwarded":true,"forwardedFromHost":"build.example.com"}""",
            ),
        )!!
        assertEquals("MacBook Pro is asking to use “GitHub”", forwarded.bannerTitle)
        assertEquals("build.example.com wants to push to git.tryhisar.com", forwarded.summary)
    }

    @Test
    fun testAnApprovalPushWithoutAValidPreviewKeepsTheSentence() {
        val backend = FakeBackend()
        // No approval part: an older Mac, so the notification keeps the kind's sentence.
        assertNull(backend.openApprovalPush("""{"kind":"approval"}"""))
        // Not a push at all.
        assertNull(backend.openApprovalPush("garbage"))
        // Go rejects an expired preview or a wrong signature.
        backend.isApprovalPushValid = false
        assertNull(
            backend.openApprovalPush(
                message(
                    """{"requestId":"req-1","expiresAt":"2020-01-01T00:00:00Z","kind":"ssh","appName":"Terminal","purpose":"push to git.tryhisar.com","keyName":"GitHub"}""",
                ),
            ),
        )
    }
}
