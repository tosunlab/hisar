package com.tryhisar.hisar.autofill

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.SigningInfo
import java.security.MessageDigest

/** What the device says about the app asking to be filled. */
object InstalledApps {
    /**
     * The app with the SHA-256 of its signing certificate, or null when it cannot be read or the app has several signers,
     * whose links Hisar cannot pin to one certificate. An app whose key rotated reports its current certificate.
     */
    fun readAndroidApp(context: Context, packageName: String): AndroidApp? {
        val signingInfo = try {
            context.packageManager.getPackageInfo(packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo
        } catch (_: PackageManager.NameNotFoundException) {
            null
        } ?: return null
        return readAndroidApp(packageName, signingInfo)
    }

    fun readAndroidApp(packageName: String, signingInfo: SigningInfo): AndroidApp? {
        if (signingInfo.hasMultipleSigners()) return null
        val certificate = signingInfo.apkContentsSigners.singleOrNull() ?: return null
        val digest = MessageDigest.getInstance("SHA-256").digest(certificate.toByteArray())
        return AndroidApp(packageName, digest.joinToString(":") { "%02X".format(it) })
    }

    fun readLabel(context: Context, packageName: String): String = try {
        val packageManager = context.packageManager
        val label = packageManager.getApplicationLabel(packageManager.getApplicationInfo(packageName, 0)).toString()
        val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        val isAmbiguous = label != packageName && packageManager.queryIntentActivities(launcher, 0).any {
            it.activityInfo.packageName != packageName && it.loadLabel(packageManager).toString() == label
        }
        if (isAmbiguous) "$label ($packageName)" else label
    } catch (_: PackageManager.NameNotFoundException) {
        packageName
    }
}
