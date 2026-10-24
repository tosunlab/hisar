package com.tryhisar.hisar.passkey

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.SdkSuppress
import org.junit.Assert.assertNotEquals
import org.junit.Test
import org.junit.runner.RunWith

/** The pending intents behind the entries Hisar puts in Credential Manager's sheet. */
@SdkSuppress(minSdkVersion = 34)
@RunWith(AndroidJUnit4::class)
class CredentialEntriesTests {
    private val context = ApplicationProvider.getApplicationContext<Context>()

    // An earlier process of Hisar numbered its entries from 1 too; a sheet it filled may still be open.
    @Test
    fun testEntryOfAnEarlierProcessIsNeverRebound() {
        val earlierEntry = PendingIntent.getActivity(context, 1, Intent(context, CredentialActivity::class.java), PendingIntent.FLAG_MUTABLE)
        try {
            val entry = CredentialPick.buildPendingIntent(context, CredentialPick.Password("personal/github"))
            assertNotEquals(earlierEntry, entry)
            entry.cancel()
        } finally {
            earlierEntry.cancel()
        }
    }
}
