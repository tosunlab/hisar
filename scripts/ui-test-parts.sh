# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's median on GitHub's runners, measured on 2026-10-23:
# - iOS (macos-26): the "Test Suite '<class>' passed ... in N (M) seconds" lines, M, of the 14 green "iPhone UI tests" jobs of
#   macOS and iOS runs 39851893409, 39871406706, 39874975576, 39899514473, 39902774426, 39905934704 and 39909260176.
#   WideLayoutUITests: one run on a Mac's iPhone 17 Pro simulator on 2026-10-27 (118 s), until CI has run it.
# - Android (ubuntu-latest, API 36 emulator): each test's wall time, from its "TestRunner: started" logcat line to the next
#   test's, so the orchestrator's 2.3 s between tests counts too; the median per test added up per class. From the 14
#   android-reports-<part> artifacts of CI runs 40595449825, 40599404870, 40661846716, 40611285397, 40611314192,
#   40645082873, 40645263609 and 40665680491 (2026-10-27 and 28). ui.AccountLifecycleUITests, ui.BiometricUITests,
#   ui.item.ItemEditorFieldNameTests and platform.BiometricEnrollmentTests are estimates from classes like them, until CI
#   has measured them.
# Retained-class estimates account for the tests and steps removed on 2026-10-28. HisarUITests and AccountLifecycleUITests
# scale the audited reductions to the older iOS weights; PhoneApprovalUITests uses its 102.4 s local after measurement.
# Android reductions include each deleted test's audited body time and the 2.3 s transition, or two transitions for the modal merge.
# Measure again when a part grows well past the others.
# The new onboarding classes use provisional 120 s weights and CredentialRevealUITests uses 60 s until CI measures them.
IOS_UI_TEST_SECONDS=(
  HisarUITests=390 OrganizationUITests=226 FavoritesUITests=176 SharingUITests=173 AccountLifecycleUITests=133
  WideLayoutUITests=118 PasskeyUITests=109 PhoneApprovalUITests=102 OneTimeCodeUITests=95 FaceIDUITests=83
  CredentialRevealUITests=60 OnboardingUITests=120
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  autofill.AutofillServiceUITests=260 passkey.PasskeyUITests=221 ui.VaultsScreenUITests=176
  AccountItemFlowUITests=136 PhoneApprovalUITests=94 FileHandoffUITests=115 SharingUITests=100
  ui.item.ItemDetailScreenTests=92 OrganizationUITests=96 AutoLockUITests=93 ui.item.ItemEditorUITests=86
  platform.ClipboardAdapterTests=66 ui.CredentialImportUITests=49 CrossDeviceSyncUITests=54
  VaultManagementUITests=52 ui.RootScreenUITests=37 FavoritesUITests=50 autofill.OneTimeCodeUITests=50
  ui.AccountLifecycleUITests=50 ui.signin.QrScannerTests=47 ui.EditorFieldHygieneTests=46
  ui.item.ItemDetailUITests=46 PrivacyUITests=40 ui.BiometricUITests=40
  ui.item.PasswordGeneratorSheetUITests=38
  ui.settings.CredentialImportCancellationUITests=31 ui.DevicesUITests=30 ui.FieldAccessibilityTests=24
  ui.DeepLinkUITests=21 ui.signin.SignInStateTests=21 ui.InvitationMenuUITests=19
  autofill.AutoFillSettingsUITests=18 ProviderScreenshotTests=16 backend.HisarKitBackendTests=16
  ui.sharing.ModalSheetBarTests=11 passkey.CredentialCompletionTests=15
  ui.item.PasswordGeneratorControlTests=14 ui.UpdateRequiredScreenTests=9 platform.BiometricEnrollmentTests=8
  ui.item.ItemEditorFieldNameTests=7 ui.settings.PendingRecoveryStateTests=7 ui.approvals.ApprovalDetailTests=5
  platform.TrustedCertificatesTests=4 platform.KeystoreSecretKeyStoreTests=3
  platform.BiometricKeystoreTests=2 platform.StateDirectoryTests=2 ui.item.IconPhotoConverterTests=2
  autofill.DatasetsTests=1 passkey.CredentialEntriesTests=1 ui.settings.PasswordCharacterRuntimeTests=1
  SharingRefreshTests=17 autofill.AutoFillUITests=21 passkey.PasskeyLayoutTests=3
  ui.BrowseRowTests=2 ui.BrowseStateTests=9 ui.BusyButtonSemanticsTests=2 ui.CredentialRevealTests=13
  ui.ExportCopyUITests=7 ui.HeadingSemanticsTests=9 ui.item.EditorSaveFeedbackTests=4
  ui.item.IconPickerSemanticsTests=3 ui.item.ItemDetailFailureTests=11 ui.item.ItemDetailOverflowTests=11
  ui.item.OtherFieldEditorTests=2 ui.lock.UnlockPanelFocusTests=2 ui.root.MainTabsTests=3
  ui.settings.ChoiceGroupScreenTests=8 ui.settings.ChoiceSemanticsTests=2 ui.settings.CredentialImportPreviewTests=3
  ui.settings.ImportFileNameLayoutTests=2 ui.settings.NavigationRowLayoutTests=4
  OnboardingUITests=120 OnboardingServerUITests=120
)

# ui_test_part PART COUNT CLASS=SECONDS... sets UI_TEST_PART_CLASSES. For a part below COUNT, these are the classes it runs;
# for part COUNT, they are the classes of the other parts, which it leaves out.
ui_test_part() {
  local part=$1 count=$2 class seconds best index
  local -a load=()
  shift 2
  UI_TEST_PART_CLASSES=()
  for ((index = 1; index <= count; index++)); do load[index]=0; done
  while IFS='=' read -r class seconds; do
    best=1
    for ((index = 2; index <= count; index++)); do
      if [ "${load[index]}" -lt "${load[best]}" ]; then best=$index; fi
    done
    load[best]=$((load[best] + seconds))
    if [ "$part" -lt "$count" ] && [ "$best" -eq "$part" ]; then UI_TEST_PART_CLASSES+=("$class"); fi
    if [ "$part" -eq "$count" ] && [ "$best" -ne "$count" ]; then UI_TEST_PART_CLASSES+=("$class"); fi
  done < <(printf '%s\n' "$@" | LC_ALL=C sort -t= -k2,2nr -k1,1)
}
