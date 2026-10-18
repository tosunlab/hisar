package com.tryhisar.hisar.passkey

import android.content.Context
import android.os.Build
import androidx.annotation.RequiresApi
import androidx.credentials.provider.CallingAppInfo
import com.tryhisar.hisar.autofill.InstalledApps
import com.tryhisar.hisar.autofill.PrivilegedBrowserList

/** Turns the platform's description of a Credential Manager caller into a [CredentialCaller], or refuses it. */
@RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
object CredentialCallers {
    /**
     * A browser that passed a web origin must be on Hisar's privileged list (Google's, merged with the community list
     * Bitwarden and KeePassDX ship): for one that is not, getOrigin throws, and the caller is refused. Any other caller is
     * an app named by its signing certificate; an app with several signers is refused.
     */
    fun read(context: Context, callingAppInfo: CallingAppInfo): CredentialCaller? {
        if (callingAppInfo.isOriginPopulated()) {
            val origin = try {
                callingAppInfo.getOrigin(PrivilegedBrowserList.readText(context))
            } catch (_: IllegalStateException) {
                null
            }
            return origin?.let { CredentialCaller.Browser(callingAppInfo.packageName, it) }
        }
        val app = InstalledApps.readAndroidApp(callingAppInfo.packageName, callingAppInfo.signingInfo) ?: return null
        return CredentialCaller.App(app, InstalledApps.readLabel(context, app.packageName))
    }
}
