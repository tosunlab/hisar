# shellcheck shell=bash disable=SC2034
# Sourced by scripts/test-ios-ui.sh and scripts/test-android-ui.sh: splits the UI test classes into CI parts of similar
# length. ui_test_part takes the classes longest first and gives each to the part with the least time so far. The last part
# runs every class the other parts do not, so a class missing from a table, such as a new one, runs there.
#
# The seconds are each class's median on GitHub's runners, measured on 2026-10-23:
# - iOS (macos-26): the "Test Suite '<class>' passed ... in N (M) seconds" lines, M, of the 14 green "iPhone UI tests" jobs of
#   macOS and iOS runs 39851893409, 39871406706, 39874975576, 39899514473, 39902774426, 39905934704 and 39909260176.
#   WideLayoutUITests: one run on a Mac's iPhone 17 Pro simulator on 2026-10-27 (118 s), until CI has run it.
# - Android (ubuntu-latest, API 36 emulator): the testcase times in the JUnit XML of 69 android-reports-<part> artifacts of
#   48 CI runs from 2026-10-22 and 2026-10-23, passing tests only, the median per test added up per class. Classes under
#   half a second, and those only API 37 runs, are left out. Classes added or grown since, measured on 2026-10-28: the
#   test times of CI run 40641727162 where it reached them (ui.BrowseRowTests, ui.BrowseStateTests,
#   ui.BusyButtonSemanticsTests, ui.CredentialImportUITests, ui.CredentialRevealTests, ui.ExportCopyUITests,
#   ui.HeadingSemanticsTests, passkey.PasskeyLayoutTests), otherwise twice their time on a Mac's API 36 emulator.
# Measure again when a part grows well past the others. An Android part is also bounded by the runner's memory: the
# emulator's host process grew by about 85 MB with each test in run 40641727162 and exhausted its 16 GB runner after
# 80 tests while idle Gradle/Kotlin build daemons held about 6.7 GB. test-android-ui.sh now releases the build JVMs
# before running tests, but emulator growth still needs headroom when adding classes.
IOS_UI_TEST_SECONDS=(
  HisarUITests=417 OrganizationUITests=226 FavoritesUITests=176 SharingUITests=173 AccountLifecycleUITests=143
  AutoFillUITests=132 WideLayoutUITests=118 PasskeyUITests=109 PhoneApprovalUITests=107 OneTimeCodeUITests=95 FaceIDUITests=83
)
# Classes of the package com.tryhisar.hisar.
ANDROID_UI_TEST_SECONDS=(
  AutoLockUITests=141 ui.VaultsScreenUITests=138 PhoneApprovalUITests=121 AccountItemFlowUITests=118
  autofill.AutofillServiceUITests=110 FileHandoffUITests=109 OrganizationUITests=91 passkey.PasskeyUITests=81
  SharingUITests=78 ui.item.ItemEditorUITests=78 VaultManagementUITests=50 ui.AccountLifecycleUITests=48
  platform.ClipboardAdapterTests=46 CrossDeviceSyncUITests=44 autofill.OneTimeCodeUITests=43 ui.RootScreenUITests=43
  FavoritesUITests=42 PrivacyUITests=41 ui.EditorFieldHygieneTests=41 ui.CredentialImportUITests=39
  ui.item.ItemDetailScreenTests=35 ui.item.ItemDetailUITests=34 ui.item.PasswordGeneratorSheetUITests=34
  ui.settings.CredentialImportCancellationUITests=30 autofill.AutoFillUITests=21 ui.signin.QrScannerTests=20
  SharingRefreshTests=17 ui.DeepLinkUITests=16 ui.InvitationMenuUITests=16 ui.item.ItemDetailClipboardTests=14
  ui.CredentialRevealTests=13 autofill.AutoFillSettingsUITests=12 backend.HisarKitBackendTests=12
  ui.item.ItemDetailFailureTests=11 ui.item.ItemDetailOverflowTests=11 ProviderScreenshotTests=10 ui.BrowseStateTests=9
  ui.HeadingSemanticsTests=9 ui.DevicesUITests=8 ui.settings.ChoiceGroupScreenTests=8 ui.ExportCopyUITests=7
  ui.signin.SignInStateTests=7 HarnessServerTests=5 ui.item.EditorSaveFeedbackTests=4
  ui.settings.NavigationRowLayoutTests=4 passkey.PasskeyLayoutTests=3 platform.TrustedCertificatesTests=3
  ui.item.IconPickerSemanticsTests=3 ui.root.MainTabsTests=3 ui.settings.CredentialImportPreviewTests=3
  ui.settings.PendingRecoveryStateTests=3 ui.BrowseRowTests=2 ui.BusyButtonSemanticsTests=2
  ui.item.OtherFieldEditorTests=2 ui.lock.UnlockPanelFocusTests=2 ui.settings.ChoiceSemanticsTests=2
  ui.settings.ImportFileNameLayoutTests=2 passkey.CredentialCompletionTests=1 ui.approvals.ApprovalDetailTests=1
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
