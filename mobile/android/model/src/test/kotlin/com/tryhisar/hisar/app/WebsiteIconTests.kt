package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ChosenIcon
import com.tryhisar.hisar.model.ItemDetail
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.io.bytestring.ByteString

class WebsiteIconTests {
    @Test
    fun testOnlyLoginsWithAnHTTPSWebsiteAndNoIconShowAWebsiteIcon() {
        assertEquals("https://github.com/login", ItemDetail(allURL = listOf("github.com", " https://github.com/login ")).websiteIconURL)
        assertNull(ItemDetail(allURL = listOf("http://github.com")).websiteIconURL)
        assertNull(ItemDetail(allURL = listOf("https://github.com"), icon = ChosenIcon(preset = "code")).websiteIconURL)
        assertNull(ItemDetail(category = ItemDetail.CATEGORY_SERVER, allURL = listOf("https://admin.example.com")).websiteIconURL)
    }

    @Test
    fun testWebsiteIconsLoadOncePerDomainAndFollowTheSetting() = runTest {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.websiteIconByURL = mutableMapOf("https://github.com/login" to ByteString(9), "https://github.com/" to ByteString(9))
        val model = harness.startUnlockedModel()
        model.websiteIcons.loadWebsiteIcon("https://github.com/login")
        model.websiteIcons.loadWebsiteIcon("https://github.com/")
        runCurrent()
        assertEquals(ByteString(9), model.websiteIcons.getWebsiteIcon("https://GitHub.com/settings"))
        assertEquals(1, backend.allCallName.count { it == "getWebsiteIcon" })

        model.lock()
        assertNull(model.websiteIcons.getWebsiteIcon("https://github.com/"), "locking forgets the icons")

        backend.seedSignedIn()
        model.refreshAll()
        model.websiteIcons.setWebsiteIconsEnabled(false)
        assertFalse(model.isWebsiteIconsEnabled.value)
        assertFalse(backend.isWebsiteIconsEnabledValue)
        model.websiteIcons.loadWebsiteIcon("https://github.com/")
        runCurrent()
        assertEquals(1, backend.allCallName.count { it == "getWebsiteIcon" }, "nothing is asked with website icons off")
    }
}
