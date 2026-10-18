package com.tryhisar.hisar.platform

import android.content.pm.ApplicationInfo
import android.system.Os
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.xmlpull.v1.XmlPullParser

@RunWith(AndroidJUnit4::class)
class StateDirectoryTests {
    @Test
    fun testNoBackupDirectoryHoldsTheState() {
        val stateDirectory = StateDirectory.prepare(targetContext)
        assertTrue(stateDirectory.isDirectory)
        assertEquals("Android never backs up or moves the no-backup directory", targetContext.noBackupFilesDir, stateDirectory.parentFile)
        assertEquals("only Hisar reads the state", "700".toInt(8), Os.stat(stateDirectory.path).st_mode and "777".toInt(8))
    }

    @Test
    fun testBackupRulesExcludeTheState() {
        assertEquals("no cloud backup through adb or Android 11 and earlier", 0, targetContext.applicationInfo.flags and ApplicationInfo.FLAG_ALLOW_BACKUP)
        val application = readInstalledApplicationAttributes()
        assertEquals(R.xml.data_extraction_rules, application["dataExtractionRules"])
        assertEquals(R.xml.backup_rules, application["fullBackupContent"])

        assertEquals("Android 12 and later back up and transfer nothing", mapOf("cloud-backup" to allDomain, "device-transfer" to allDomain), readRules(R.xml.data_extraction_rules))
        assertEquals("Android 11 and earlier back up nothing", mapOf("full-backup-content" to allDomain), readRules(R.xml.backup_rules))
    }

    /** The rules' sections, each with the domains it excludes whole; an include anywhere fails the test. */
    private fun readRules(resource: Int): Map<String, Set<String>> {
        val allExcludedDomainBySection = mutableMapOf<String, MutableSet<String>>()
        var section = ""
        targetContext.resources.getXml(resource).use { parser ->
            while (parser.next() != XmlPullParser.END_DOCUMENT) {
                if (parser.eventType != XmlPullParser.START_TAG) continue
                when (parser.name) {
                    "cloud-backup", "device-transfer", "full-backup-content" -> section = parser.name
                    "exclude" -> {
                        assertEquals("a rule excludes a whole domain", ".", parser.getAttributeValue(null, "path"))
                        allExcludedDomainBySection.getOrPut(section) { mutableSetOf() } += parser.getAttributeValue(null, "domain")
                    }
                    "data-extraction-rules" -> Unit
                    else -> throw AssertionError("unexpected <${parser.name}> in the backup rules")
                }
            }
        }
        return allExcludedDomainBySection
    }

    /** The android: attributes of <application> in the installed app's manifest, as resource IDs. */
    private fun readInstalledApplicationAttributes(): Map<String, Int> {
        targetContext.assets.openXmlResourceParser("AndroidManifest.xml").use {
            while (it.next() != XmlPullParser.END_DOCUMENT) {
                if (it.eventType == XmlPullParser.START_TAG && it.name == "application") {
                    return (0 until it.attributeCount).associate { index -> it.getAttributeName(index) to it.getAttributeResourceValue(index, 0) }
                }
            }
        }
        throw AssertionError("the manifest has no <application>")
    }

    private companion object {
        val allDomain = setOf("root", "file", "database", "sharedpref", "external", "device_root", "device_file", "device_database", "device_sharedpref")
    }
}
