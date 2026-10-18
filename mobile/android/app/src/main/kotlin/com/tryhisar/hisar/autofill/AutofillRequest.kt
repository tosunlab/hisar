package com.tryhisar.hisar.autofill

import android.content.Intent
import android.view.autofill.AutofillId
import androidx.core.content.IntentCompat

/**
 * What the AutoFill screens fill, carried from the service to [AutofillActivity] in a suggestion's authentication intent:
 * the form's fields, the site or app, and for a suggestion of one login, that login and how it matched.
 */
data class AutofillRequest(
    val username: AutofillId?,
    val password: AutofillId?,
    val oneTimeCode: AutofillId?,
    val kind: RequestKind,
    val target: FillTarget,
    val pick: Pick? = null,
) {
    data class Pick(val recordIdentifier: String, val source: LoginSource)

    fun writeTo(intent: Intent): Intent = intent.apply {
        putExtra(EXTRA_USERNAME, username)
        putExtra(EXTRA_PASSWORD, password)
        putExtra(EXTRA_ONE_TIME_CODE, oneTimeCode)
        putExtra(EXTRA_KIND, kind.name)
        when (target) {
            is FillTarget.Websites -> putExtra(EXTRA_WEBSITES, target.allServiceIdentifier.toTypedArray())
            is FillTarget.App -> {
                putExtra(EXTRA_APP_PACKAGE, target.app.packageName)
                putExtra(EXTRA_APP_CERTIFICATE, target.app.certificateSHA256)
                putExtra(EXTRA_APP_LABEL, target.label)
            }
        }
        if (pick != null) {
            putExtra(EXTRA_RECORD, pick.recordIdentifier)
            when (val source = pick.source) {
                is LoginSource.Website -> putExtra(EXTRA_SOURCE_WEBSITE, source.serviceIdentifier)
                is LoginSource.App -> putExtra(EXTRA_SOURCE_APP, true)
            }
        }
    }

    companion object {
        private const val EXTRA_USERNAME = "com.tryhisar.hisar.autofill.USERNAME"
        private const val EXTRA_PASSWORD = "com.tryhisar.hisar.autofill.PASSWORD"
        private const val EXTRA_ONE_TIME_CODE = "com.tryhisar.hisar.autofill.ONE_TIME_CODE"
        private const val EXTRA_KIND = "com.tryhisar.hisar.autofill.KIND"
        private const val EXTRA_WEBSITES = "com.tryhisar.hisar.autofill.WEBSITES"
        private const val EXTRA_APP_PACKAGE = "com.tryhisar.hisar.autofill.APP_PACKAGE"
        private const val EXTRA_APP_CERTIFICATE = "com.tryhisar.hisar.autofill.APP_CERTIFICATE"
        private const val EXTRA_APP_LABEL = "com.tryhisar.hisar.autofill.APP_LABEL"
        private const val EXTRA_RECORD = "com.tryhisar.hisar.autofill.RECORD"
        private const val EXTRA_SOURCE_WEBSITE = "com.tryhisar.hisar.autofill.SOURCE_WEBSITE"
        private const val EXTRA_SOURCE_APP = "com.tryhisar.hisar.autofill.SOURCE_APP"

        /** The request an intent carries, or null for an intent the service did not build. */
        fun read(intent: Intent): AutofillRequest? {
            val kind = intent.getStringExtra(EXTRA_KIND)?.let { name -> RequestKind.entries.firstOrNull { it.name == name } } ?: return null
            val appPackage = intent.getStringExtra(EXTRA_APP_PACKAGE)
            val target = if (appPackage != null) {
                val certificate = intent.getStringExtra(EXTRA_APP_CERTIFICATE) ?: return null
                FillTarget.App(AndroidApp(appPackage, certificate), intent.getStringExtra(EXTRA_APP_LABEL) ?: appPackage)
            } else {
                FillTarget.Websites(intent.getStringArrayExtra(EXTRA_WEBSITES)?.toList() ?: return null)
            }
            val pick = intent.getStringExtra(EXTRA_RECORD)?.let { recordIdentifier ->
                val website = intent.getStringExtra(EXTRA_SOURCE_WEBSITE)
                val source = when {
                    website != null -> LoginSource.Website(website)
                    intent.getBooleanExtra(EXTRA_SOURCE_APP, false) && target is FillTarget.App -> LoginSource.App(target.app)
                    else -> return null
                }
                Pick(recordIdentifier, source)
            }
            return AutofillRequest(
                IntentCompat.getParcelableExtra(intent, EXTRA_USERNAME, AutofillId::class.java),
                IntentCompat.getParcelableExtra(intent, EXTRA_PASSWORD, AutofillId::class.java),
                IntentCompat.getParcelableExtra(intent, EXTRA_ONE_TIME_CODE, AutofillId::class.java),
                kind, target, pick,
            )
        }
    }
}
