package com.tryhisar.hisar

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.os.SystemClock
import android.service.credentials.BeginGetCredentialOption
import android.service.credentials.BeginGetCredentialRequest
import android.service.credentials.CallingAppInfo
import android.service.credentials.CredentialProviderService
import android.view.WindowManager
import androidx.credentials.PasswordCredential
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import com.tryhisar.hisar.autofill.AutofillActivity
import com.tryhisar.hisar.autofill.AutofillRequest
import com.tryhisar.hisar.autofill.FillTarget
import com.tryhisar.hisar.autofill.RequestKind
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.passkey.CredentialActivity
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
@SdkSuppress(minSdkVersion = 34)
class ProviderScreenshotTests {
    private val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()

    private fun <T : Activity> checkWindow(intent: Intent) {
        application.installFakeBackend(FakeBackend().apply { seedSignedIn(state = VaultStatus.STATE_LOCKED) })
        val setting = ScreenshotSetting(application)
        val original = setting.isAllowed
        try {
            setting.isAllowed = true
            ActivityScenario.launch<T>(intent).use { scenario ->
                fun awaitSecure(expected: Boolean) {
                    val deadline = SystemClock.uptimeMillis() + 5_000
                    var secure = !expected
                    while (secure != expected && SystemClock.uptimeMillis() < deadline) {
                        scenario.onActivity { secure = it.window.attributes.flags and WindowManager.LayoutParams.FLAG_SECURE != 0 }
                        if (secure != expected) SystemClock.sleep(10)
                    }
                    assertEquals("the open provider window follows Allow screenshots", expected, secure)
                }
                awaitSecure(false)
                setting.isAllowed = false
                awaitSecure(true)
                setting.isAllowed = true
                awaitSecure(false)
            }
        } finally { setting.isAllowed = original }
    }

    @Test fun testAutoFillWindowFollowsScreenshotSettingWhileOpen() {
        val request = AutofillRequest(null, null, null, RequestKind.PASSWORD, FillTarget.Websites(listOf("github.com")))
        checkWindow<AutofillActivity>(request.writeTo(Intent(application, AutofillActivity::class.java)))
    }

    @Test fun testPasskeyProviderWindowFollowsScreenshotSettingWhileOpen() {
        val signingInfo = checkNotNull(application.packageManager.getPackageInfo(application.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo)
        val request = BeginGetCredentialRequest.Builder()
            .setCallingAppInfo(CallingAppInfo(application.packageName, signingInfo))
            .addBeginGetCredentialOption(BeginGetCredentialOption("password", PasswordCredential.TYPE_PASSWORD_CREDENTIAL, Bundle()))
            .build()
        val intent = Intent(application, CredentialActivity::class.java)
            .putExtra("com.tryhisar.hisar.passkey.PICK", "unlock")
            .putExtra(CredentialProviderService.EXTRA_BEGIN_GET_CREDENTIAL_REQUEST, request)
        checkWindow<CredentialActivity>(intent)
    }
}
