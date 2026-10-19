package com.tryhisar.hisar.autofill

import android.content.Context
import com.tryhisar.hisar.R

/**
 * `privileged_browsers.json`, read once for the passkey provider, which hands its text to Credential Manager, and for
 * AutoFill, which matches a caller's package and certificate against its entries.
 */
object PrivilegedBrowserList {
    @Volatile
    private var text: String? = null

    @Volatile
    private var browsers: PrivilegedBrowsers? = null

    fun readText(context: Context): String =
        text ?: context.assets.open("privileged_browsers.json").use { it.readBytes().decodeToString() }.also { text = it }

    /**
     * The debug build also trusts the pages of its instrumented tests' APK, which is signed with Hisar's own certificate:
     * Android runs instrumentation only from an APK signed like the app under test.
     */
    fun read(context: Context): PrivilegedBrowsers = browsers ?: run {
        val ownCertificate = InstalledApps.readAndroidApp(context, context.packageName)?.certificateSHA256
        val allTestPackage = context.resources.getStringArray(R.array.autofill_test_browsers)
        val allWebViewApp = ownCertificate?.let { certificate -> allTestPackage.mapTo(HashSet()) { AndroidApp(it, certificate) } }.orEmpty()
        PrivilegedBrowsers.parse(readText(context), allWebViewApp)
    }.also { browsers = it }
}
