package com.tryhisar.hisar.platform

import android.system.Os
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class StateDirectoryTests {
    @Test
    fun testNoBackupDirectoryHoldsTheState() {
        val stateDirectory = StateDirectory.prepare(targetContext)
        assertTrue(stateDirectory.isDirectory)
        assertEquals("Android never backs up or moves the no-backup directory", targetContext.noBackupFilesDir, stateDirectory.parentFile)
        assertEquals("only Hisar reads the state", "700".toInt(8), Os.stat(stateDirectory.path).st_mode and "777".toInt(8))
    }
}
