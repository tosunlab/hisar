package com.tryhisar.hisar.model

import java.io.File
import kotlin.test.Test
import kotlin.test.fail

/**
 * Every unit and UI test of the iPhone app has an Android test of the same name, a name listed in [renamed], or a reason in
 * [notApplicable]. Tests not ported yet are listed in ios-test-parity-expected-missing.txt, which only shrinks: porting a
 * test fails this check until its line is removed, and a new iPhone test fails it until it is ported.
 */
class IosTestParityTest {
    private val renamed = mapOf(
        "testFaceIDUnlockAfterTurningItOn" to "testBiometricUnlockAfterTurningItOn",
        "testApproveSSHRequestWithFaceID" to "testApproveSSHRequestWithBiometrics",
        "testApproveWithMasterPasswordWhenFaceIDIsUnavailable" to "testApproveWithMasterPasswordWhenBiometricsAreUnavailable",
        "testSharedAccessGroupIsUsedInTheSimulator" to "testSecretKeyIsEncryptedWithAKeystoreKey",
        "testAppGroupContainerHoldsTheState" to "testNoBackupDirectoryHoldsTheState",
        "testPersonalTeamFallsBackToApplicationSupport" to "testBackupRulesExcludeTheState",
        "testAppPublishesPasskeyIdentitiesAndDeletesPasskeys" to "testAppDeletesPasskeys",
        "testSecretKeyScreenOffersFaceIDAndTurnsItOnInOneTap" to "testSecretKeyScreenOffersBiometricsAndTurnsThemOnInOneTap",
        "testFaceIDOfferFailureStaysOnTheOffer" to "testBiometricOfferFailureStaysOnTheOffer",
        "testSecretKeyScreenSkipsTheOfferWithoutFaceID" to "testSecretKeyScreenSkipsTheOfferWithoutBiometrics",
        "testRefreshFinishingAfterLockingBringsNothingBackAndKeepsAutoFill" to "testRefreshFinishingAfterLockingBringsNothingBack",
        "testSignedOutByServerShowsReasonAndClearsAutoFill" to "testSignedOutByServerShowsReason",
        "testDeleteAccountSignsOutAndRemovesAutoFillIdentities" to "testDeleteAccountSignsOut",
    )

    private val notApplicable = run {
        val identityStore = "Android AutoFill asks Hisar for logins at fill time; it has no credential identity store to keep up to date (D3)"
        mapOf(
            "testSignOutClearsIdentities" to identityStore,
            "testRefreshPublishesCredentialIdentities" to identityStore,
            "testPasskeyIdentitiesOnlyForListedLogins" to identityStore,
            "testDeletingAnItemRemovesItsPasskeyIdentities" to identityStore,
            "testCredentialIdentitiesSkipNonLoginsAndEmptyUsernames" to identityStore,
            "testCredentialIdentitiesOfferAServerOnItsAdminConsole" to identityStore,
            "testOneTimeCodeIdentitiesForLoginsWithACode" to identityStore,
        )
    }

    private data class SwiftTest(val className: String, val name: String) {
        override fun toString() = "$className.$name"
    }

    private val repositoryDirectory = File(
        System.getProperty("hisar.repositoryRoot") ?: fail("Run this test through Gradle, which sets hisar.repositoryRoot"),
    )

    private fun listSwiftTests(): List<SwiftTest> {
        val classPattern = Regex("""^(?:final )?class (\w+)""")
        val testPattern = Regex("""^\s*func (test\w+)\(""")
        return listOf("Unit", "UI").flatMap { directory ->
            File(repositoryDirectory, "mobile/ios/Tests/$directory").listFiles { file -> file.extension == "swift" }.orEmpty().sorted()
                .flatMap { file ->
                    var className = file.nameWithoutExtension
                    file.readLines().mapNotNull { line ->
                        classPattern.find(line)?.let { className = it.groupValues[1] }
                        testPattern.find(line)?.let { SwiftTest(className, it.groupValues[1]) }
                    }
                }
        }
    }

    private fun listKotlinTestNames(): Set<String> {
        val testPattern = Regex("""\bfun (test\w+)\(""")
        return File(repositoryDirectory, "mobile/android").walkTopDown()
            .onEnter { it.name != "build" && !it.name.startsWith(".") }
            .filter { it.extension == "kt" && ("/src/test/" in it.invariantSeparatorsPath || "/src/androidTest/" in it.invariantSeparatorsPath) }
            .flatMap { file -> testPattern.findAll(file.readText()).map { it.groupValues[1] } }
            .toSet()
    }

    private fun readExpectedMissing(): Set<String> {
        val resource = javaClass.getResource("/ios-test-parity-expected-missing.txt") ?: return emptySet()
        return resource.readText().lines().map { it.trim() }.filter { it.isNotEmpty() && !it.startsWith("#") }.toSet()
    }

    @Test
    fun testEveryIosTestHasAnAndroidCounterpart() {
        val allSwiftTest = listSwiftTests()
        val allSwiftName = allSwiftTest.map { it.name }.toSet()
        val allKotlinName = listKotlinTestNames()
        val allProblem = mutableListOf<String>()

        for (name in (renamed.keys + notApplicable.keys) - allSwiftName) {
            allProblem += "IosTestParityTest lists $name, which is no iPhone test; remove it from the table"
        }
        val missing = allSwiftTest
            .filter { (renamed[it.name] ?: it.name) !in allKotlinName && it.name !in notApplicable }
            .map { it.toString() }
            .toSet()
        val expectedMissing = readExpectedMissing()
        for (test in (missing - expectedMissing).sorted()) {
            val kotlinName = renamed[test.substringAfter('.')] ?: test.substringAfter('.')
            allProblem += "$test has no Android test named $kotlinName; port it, or list it in IosTestParityTest with a reason"
        }
        for (test in (expectedMissing - missing).sorted()) {
            allProblem += "$test is no longer missing; remove its line from ios-test-parity-expected-missing.txt"
        }
        if (allProblem.isNotEmpty()) {
            fail(allProblem.joinToString("\n", postfix = "\n${missing.size} iPhone tests are still missing on Android"))
        }
    }
}
