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
        "testApproveWithFaceID" to "testApproveWithBiometrics",
        "testDenyNeedsNoFaceID" to "testDenyNeedsNoBiometrics",
        "testCancelledFaceIDKeepsTheRequest" to "testCancelledBiometricsKeepTheRequest",
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
        "testSaveNewItemAddsItWithoutSecretsAndUpdatesIdentities" to "testSaveNewItemAddsItWithoutSecrets",
        "testQuickTypeRequestUnlocksThenCompletes" to "testSuggestionRequestUnlocksThenCompletes",
        "testOneTimeCodeQuickTypeRequestUnlocksThenCompletes" to "testOneTimeCodeSuggestionRequestUnlocksThenCompletes",
        "testChosenPasskeyInAnUnlockedExtensionAsksTheDeviceOwner" to "testChosenPasskeyWhileUnlockedAsksTheDeviceOwner",
        // Credential Manager asks the provider at request time, so the iPhone's identity save becomes the next request's offer.
        "testPasskeyRegistrationUpdatesTheCredentialIdentities" to "testCreatedPasskeyIsOfferedOnTheNextRequest",
        "testTokenWaitsForUnlockAndPreparesOnce" to "testTransferWaitsForUnlockAndPreparesOnce",
        "testTokenWhileSignedOutExplainsInstead" to "testImportWhileSignedOutExplainsInstead",
        // A UnifiedPush distributor gives an endpoint instead of a token, and Android 13's prompt is the app's.
        "testTurningNotificationsOnAsksOnceThenRegistersTheTokenAsHex" to "testTurningNotificationsOnNeedsAUnifiedPushDistributor",
    )

    private val notApplicable = run {
        val identityStore = "Android AutoFill asks Hisar for logins at fill time; it has no credential identity store to keep up to date (D3)"
        mapOf(
            "testCredentialErrorsStayBesideSubmission" to
                "Checks the iOS Form error position with its keyboard; Android uses a separate Compose form",
            "testSignInModesScaleAtAccessibilitySizes" to
                "Checks the iOS Dynamic Type replacement for its unscaled segmented control; Android has scaled Compose mode buttons",
            "testCameraAuthorizationChoosesActionableScannerState" to
                "Covers AVFoundation camera authorization and the iOS app Settings route",
            "testForegroundRefreshUpdatesAutoFillWithoutRequestingPermission" to
                "iOS Settings exposes the credential identity store's enabled state; Android uses its own AutoFill service status.",
            "testForegroundRefreshRegistersOnlyNewlyAllowedNotifications" to
                "Only iOS registers an APNs token when notification authorization changes; Android receives its UnifiedPush endpoint separately.",
            "testInvitationEmailWrapsLiteralCharactersAtAccessibilitySizes" to
                "UIKit-specific regression for iOS automatic hyphenation and selectable UITextView Dynamic Type wrapping; Android does not use UIKit",
            "testIconPhotoPreparationRejectsStaleResults" to
                "iOS PhotosPicker uses an asynchronous transfer and conversion task; this test covers its generation guard and sheet dismissal",
            "testTagsKeepNamesAndCountsReadableAtAccessibilitySizes" to
                "The iPhone adapts its native badge at accessibility Dynamic Type sizes; Android's tag rows use their own Compose layout",
            "testRefreshFailureKeepsReadableListAndOperationError" to
                "The iPhone's operation callback retains browse errors independently of AppModel.errorMessage; Android uses area-model refusals",
            "testDetailHeaderUsesFullWidthAtAccessibilitySizesAndCapsItsIcon" to
                "SwiftUI Dynamic Type header layout; Android uses its own Compose header and font scaling",
            "testPresetGridFitsScaledTargetsAtBothPhoneWidths" to
                "SwiftUI adaptive preset grid at iOS Dynamic Type sizes; Android uses its own Compose icon picker",
            "testRemovedVaultDismissTargetNamesItsVaultAndMeetsMinimumSize" to
                "iPhone's shared 44-point icon hit-area helper; Android uses Material minimum interactive targets",
            "testSupportingTextContrastAcrossPalettesAndAppearances" to
                "Resolves SwiftUI themeSecondary against iOS grouped Form and List backgrounds",
            "testLightSupportingInstructionsRenderInSignInExportAndAccountChange" to
                "Captures iPhone supporting-text rendering in Light appearance; Android uses Compose",
            "testPersistentEditorLabelsKeepTheirSpaceWithPopulatedAndEmptyValues" to
                "SwiftUI persistent visible editor labels; Android field labeling is covered by its Compose semantics tests",
            "testPopulatedEditorsRenderLabeledIdentityAddressCredentialLicenseServerAndDatabaseFields" to
                "Captures native SwiftUI populated editors at iOS Dynamic Type sizes; Android uses Compose",
            "testSignOutClearsIdentities" to identityStore,
            "testRefreshPublishesCredentialIdentities" to identityStore,
            "testPasskeyIdentitiesOnlyForListedLogins" to identityStore,
            "testDeletingAnItemRemovesItsPasskeyIdentities" to identityStore,
            "testCredentialIdentitiesSkipNonLoginsAndEmptyUsernames" to identityStore,
            "testCredentialIdentitiesOfferAServerOnItsAdminConsole" to
                "Only identity publication is inapplicable because Android has no credential identity store. " +
                    "VaultSearchTests.testServersAreFoundByTheirHostPortAndAddress covers the search and category assertions.",
            "testOneTimeCodeIdentitiesForLoginsWithACode" to identityStore,
            "testSignedOutStartClearsLeftoverIdentities" to identityStore,
            "testForegroundWhileSignedOutClearsLeftoverIdentities" to identityStore,
            "testLockedStartKeepsAutoFillIdentities" to identityStore,
            "testALaunchAfterNotificationsWereAllowedAsksForThisLaunchsToken" to
                "the UnifiedPush distributor hands HisarPushService its endpoint by itself; there is no APNs registration to ask for",
            "testAPushPayloadDecryptsWithThePhonesKey" to
                "the UnifiedPush connector decrypts the Web Push message itself before HisarPushService sees it, so there is no payload for Hisar to decrypt",
            "testExportDatesReachTheBackendAsSecondsSince1970" to
                "Android receives the Credential Exchange JSON as the exporter wrote it; only iOS decodes it into Dates first",
            "testReplacingTheSecretKeyUpdatesTheStoredItemInPlace" to
                "Android seals the Secret Key into an AtomicFile, which replaces the old copy in one rename; there is no keychain item to delete first",
            "testNewPrivacyWindowsUseTheSelectedTheme" to
                "Only iOS creates a separate UIKit privacy window; Android draws its privacy cover inside the themed Compose root",
            "testListsStayBesideWhatTheyOpenOnAWideScreen" to
                "The iPhone app lays Vaults, Search and Settings out as split views for the open iPhone Duo; the Android app has one layout for every width so far",
            "testAnApprovalPreviewVerifiesWithTheAccountsKey" to
                "Android checks the preview in Go through HisarKit (OpenApprovalPush); core/approval's tests cover the signature",
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
