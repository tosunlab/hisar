package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.CalendarDate
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemSoftwareLicense
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.model.FakeBackend
import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.runCurrent
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** Saving, deleting and describing items, the dates and warnings the editor shows, and the generator. */
class ItemEditorModelTests {
    private suspend fun TestScope.createUnlockedModel(): AppModel = AppModelHarness(this).startUnlockedModel()

    private fun AppModel.findListed(vaultID: String, itemID: String): ItemDetail? =
        unlockedData.value.listAllItemReference(vaultID).firstOrNull { it.item.id == itemID }?.item

    private fun AppModel.listTitles(include: (ItemDetail) -> Boolean): List<String> =
        unlockedData.value.listAllItemReference().map { it.item }.filter(include).map { it.title }

    @Test
    fun testItemEditorDraftSurvivesLockAndUnlock() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val request = EditorRequest.New("personal", ItemDetail(title = "Draft", password = "generated-password"))
        model.itemEditor.open(request)
        val draft = assertNotNull(model.itemEditor.session.value).draft.copy(allURLText = listOf("https://example.com", ""), newTagText = "pending")
        model.itemEditor.updateDraft(draft)
        model.itemEditor.chooseVault("family")
        model.navigation.search.push(VaultsDestination.Editor(request, opensSavedItem = false))
        model.handleDidEnterBackground()
        harness.currentDate += model.autoLockInterval.plusSeconds(1)
        model.handleWillEnterForeground()
        assertTrue(model.navigation.search.path.value.isEmpty())
        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        val restored = assertNotNull(model.itemEditor.session.value)
        assertEquals(draft, restored.draft)
        assertEquals("family", restored.chosenVaultID)
        assertEquals(request, (model.navigation.vaults.path.value.last() as VaultsDestination.Editor).request)
    }

    @Test
    fun testItemEditorPermissionChangeKeepsDraftForRecovery() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val request = EditorRequest.New("family", ItemDetail(title = "Draft", password = "keep-this"))
        model.itemEditor.open(request)
        model.navigation.vaults.push(VaultsDestination.Editor(request, false))
        model.lock()
        harness.backend.allVault = harness.backend.allVault.map { if (it.id == "family") it.copy(role = "reader") else it }.toMutableList()
        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertNotNull(model.itemEditor.session.value?.recoveryMessage)
        assertEquals("keep-this", model.itemEditor.session.value?.draft?.item?.password)
        model.itemEditor.recoverCopy()
        assertNull(model.itemEditor.session.value?.recoveryMessage)
        assertEquals("personal", model.itemEditor.session.value?.chosenVaultID)
    }

    @Test
    fun testItemEditorChangedSourceRequiresRecovery() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val request = EditorRequest.Edit("personal", "github")
        model.itemEditor.open(request)
        val draft = assertNotNull(model.itemEditor.session.value).draft.withItem { it.copy(password = "unsaved") }
        model.itemEditor.updateDraft(draft)
        model.navigation.vaults.push(VaultsDestination.Editor(request, false))
        model.lock()
        harness.backend.allItemByVaultID.getValue("personal").removeAll { it.id == "github" }
        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertNotNull(model.itemEditor.session.value?.recoveryMessage)
        assertEquals("unsaved", model.itemEditor.session.value?.draft?.item?.password)
        model.itemEditor.recoverCopy()
        assertNull(model.itemEditor.session.value?.draft?.original)
        assertEquals("", model.itemEditor.session.value?.draft?.item?.id)
    }

    @Test
    fun testItemEditorClearsOnSaveDiscardSignOutAndAccountSwitch() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val request = EditorRequest.New("personal", ItemDetail(title = "Draft", password = "secret"))
        model.itemEditor.open(request)
        model.itemEditor.saveItem("personal", request.draft)
        assertNull(model.itemEditor.session.value)
        model.itemEditor.open(request)
        model.itemEditor.discard()
        assertNull(model.itemEditor.session.value)
        model.itemEditor.open(request)
        model.applyStatus(model.status.value.copy(email = "other@example.com"))
        assertNull(model.itemEditor.session.value)
        model.itemEditor.open(request)
        model.lock()
        model.lock.signOut()
        assertNull(model.itemEditor.session.value)
    }

    @Test
    fun testAutoFillRequestedDraftKeepsLaterEditsAndItsSaveCallback() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        var savedPassword: String? = null
        val request = EditorRequest.New("personal", ItemDetail(title = "Autofill", password = "original"), onSaved = { _, item -> savedPassword = item.password })
        model.requestedEditor.value = request
        runCurrent()
        model.itemEditor.open(request)
        val draft = assertNotNull(model.itemEditor.session.value).draft.withItem { it.copy(password = "changed-after-autofill") }
        model.itemEditor.updateDraft(draft)
        model.lock()
        model.lock.unlock(FakeBackend.PASSWORD)
        runCurrent()
        assertEquals(draft, model.itemEditor.session.value?.draft)
        val saved = assertNotNull(model.itemEditor.saveItem("personal", draft.buildItem()))
        request.onSaved?.invoke("personal", saved)
        assertEquals("changed-after-autofill", savedPassword)
    }

    @Test
    fun testSaveNewItemAddsItWithoutSecrets() = runTest {
        val model = createUnlockedModel()
        val draft = ItemDetail(title = "Mastodon", username = "alice", allURL = listOf("mastodon.social"), password = "secret")
        val savedItem = model.itemEditor.saveItem("personal", draft)
        assertEquals("secret", savedItem?.password)
        val summary = model.unlockedData.value.listAllItemReference("personal").firstOrNull { it.item.title == "Mastodon" }
        assertNotNull(summary)
        assertEquals("", summary.item.password, "list summaries never keep the password")
    }

    @Test
    fun testInvalidOneTimePasswordShowsTheDesktopMessage() = runTest {
        val model = createUnlockedModel()
        val savedItem = model.itemEditor.saveItem("personal", ItemDetail(title = "Broken", oneTimePassword = "not a secret!"))
        assertNull(savedItem)
        assertEquals("The one-time code is not a valid secret or otpauth:// link", model.errorMessage.value)
    }

    @Test
    fun testSaveWithoutTitleShowsError() = runTest {
        val model = createUnlockedModel()
        val savedItem = model.itemEditor.saveItem("personal", ItemDetail(title = "  "))
        assertNull(savedItem)
        assertEquals("Enter a title", model.errorMessage.value)
    }

    @Test
    fun testEditExistingItemReplacesIt() = runTest {
        val model = createUnlockedModel()
        val item = assertNotNull(model.itemEditor.loadItem("personal", "github"), "item missing")
        assertEquals("github-password", item.password)
        model.itemEditor.saveItem("personal", item.copy(title = "GitHub Work"))
        assertEquals(listOf("Bank", "GitHub Work"), model.unlockedData.value.listAllItemReference("personal").map { it.item.title })
    }

    @Test
    fun testDeleteItemRemovesIt() = runTest {
        val model = createUnlockedModel()
        val isDeleted = model.itemEditor.deleteItem("family", "netflix")
        assertTrue(isDeleted)
        assertTrue(model.unlockedData.value.listAllItemReference("family").isEmpty())
    }

    @Test
    fun testSaveSecureNoteKeepsItsCategoryAndIsNotALogin() = runTest {
        val model = createUnlockedModel()
        val draft = ItemDetail(category = ItemDetail.CATEGORY_SECURE_NOTE, title = "Recovery codes", notes = "one\ntwo")
        val savedItem = assertNotNull(model.itemEditor.saveItem("personal", draft), "note not saved")
        assertTrue(savedItem.isSecureNote)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals("", listed?.notes)
        assertEquals(true, listed?.isSecureNote)
        assertEquals(listOf("Recovery codes"), model.listTitles { it.isSecureNote })
        assertFalse(model.listTitles { it.isLogin }.contains("Recovery codes"))
    }

    @Test
    fun testSaveCreditCardShowsBrandAndLastFourWithoutSecrets() = runTest {
        val model = createUnlockedModel()
        val card = ItemCard(number = "4111 1111 1111 1111", securityCode = "123")
        val draft = ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD, title = "Visa", card = card)
        val savedItem = assertNotNull(model.itemEditor.saveItem("personal", draft), "card not saved")
        assertTrue(savedItem.isCreditCard)
        assertEquals("123", savedItem.card?.securityCode)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals("Visa •••• 1111", listed?.card?.summary)
        assertEquals("", listed?.card?.number)
        assertEquals("", listed?.card?.securityCode)
        assertEquals(listOf("Visa"), model.listTitles { it.isCreditCard })
        assertFalse(model.listTitles { it.isLogin }.contains("Visa"))
    }

    @Test
    fun testCardTypeIsFilledInFromTheNumberAndAChosenOneIsKept() = runTest {
        val model = createUnlockedModel()
        val card = ItemCard(number = "4111 1111 1111 1111")
        val filled = assertNotNull(model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD, title = "Visa", card = card)))
        assertEquals("Visa", filled.card?.type)
        val chosen = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_CREDIT_CARD, title = "Troy", card = card.copy(type = "Troy"))),
        )
        assertEquals("Troy •••• 1111", model.findListed("personal", chosen.id)?.card?.summary)
    }

    @Test
    fun testSaveIdentityKeepsOnlyTheNameAndEmailInTheList() = runTest {
        val model = createUnlockedModel()
        val identity = ItemIdentity(firstName = "Alice", lastName = "Example", email = "alice@example.com", passportNumber = "U1234567", address = "1 Main St")
        val savedItem = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_IDENTITY, title = "Passport", identity = identity)),
            "identity not saved",
        )
        assertTrue(savedItem.isIdentity)
        assertEquals("U1234567", savedItem.identity?.passportNumber)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals("Alice Example", listed?.identity?.summary)
        assertEquals("alice@example.com", listed?.identity?.email)
        assertEquals("", listed?.identity?.passportNumber)
        assertEquals("", listed?.identity?.address)
        assertEquals(listOf("Passport"), model.listTitles { it.isIdentity })
        assertFalse(model.listTitles { it.isLogin }.contains("Passport"))
    }

    @Test
    fun testSaveAPICredentialKeepsOnlyTheServiceKeyIDAndHostInTheList() = runTest {
        val model = createUnlockedModel()
        val credential = ItemAPICredential(service = "Stripe", keyID = "key_1", secret = "sk_live_123", host = "api.stripe.com", expiry = "2031-01-01")
        val savedItem = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_API_CREDENTIAL, title = "Billing key", apiCredential = credential)),
            "API credential not saved",
        )
        assertTrue(savedItem.isAPICredential)
        assertEquals("sk_live_123", savedItem.apiCredential?.secret)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals("Stripe · api.stripe.com", listed?.apiCredential?.summary)
        assertEquals("", listed?.apiCredential?.secret)
        assertEquals("key_1", listed?.apiCredential?.keyID)
        assertEquals("", listed?.apiCredential?.expiry)
        assertEquals(listOf("Billing key"), model.listTitles { it.isAPICredential })
        assertFalse(model.listTitles { it.isLogin }.contains("Billing key"))
    }

    @Test
    fun testSaveServerKeepsItsLoginDataAndOnlyHostAndPortInTheList() = runTest {
        val model = createUnlockedModel()
        val server = ItemServer(host = "build.example.com", port = "2222", sshKeyID = "key-1")
        val item = ItemDetail(
            category = ItemDetail.CATEGORY_SERVER, title = "Build server", username = "deploy", allURL = listOf("https://console.example.com"),
            password = "server-password", server = server,
        )
        val savedItem = assertNotNull(model.itemEditor.saveItem("personal", item), "server not saved")
        assertTrue(savedItem.isServer)
        assertTrue(savedItem.offersLoginFill)
        assertEquals(server, savedItem.server)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals(ItemServer(host = "build.example.com", port = "2222"), listed?.server)
        assertEquals("build.example.com:2222 · deploy", listed?.server?.summary(listed.username))
        assertEquals(listOf("Build server"), model.listTitles { it.isServer })
    }

    @Test
    fun testSaveDatabaseKeepsItsLoginDataAndNoConnectionStringInTheList() = runTest {
        val model = createUnlockedModel()
        val database = ItemDatabase(
            engine = "postgresql", engineName = "PostgreSQL", host = "db.example.com", port = "5432", name = "app", options = "sslmode=require",
            connectionString = "postgresql://app:db-password@db.example.com:5432/app?sslmode=require",
        )
        val item = ItemDetail(category = ItemDetail.CATEGORY_DATABASE, title = "Prod db", username = "app", password = "db-password", database = database)
        val savedItem = assertNotNull(model.itemEditor.saveItem("personal", item), "database not saved")
        assertTrue(savedItem.isDatabase)
        assertFalse(savedItem.offersLoginFill)
        assertEquals(database, savedItem.database)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals(ItemDatabase(engine = "postgresql", engineName = "PostgreSQL", host = "db.example.com", port = "5432", name = "app"), listed?.database)
        assertEquals("PostgreSQL · db.example.com:5432/app", listed?.database?.summary)
        assertEquals(listOf("Prod db"), model.listTitles { it.isDatabase })
        assertFalse(model.listTitles { it.isLogin }.contains("Prod db"))
    }

    @Test
    fun testSaveSoftwareLicenseKeepsOnlyItsSummaryInTheList() = runTest {
        val model = createUnlockedModel()
        val license = ItemSoftwareLicense(
            product = "Sketch", version = "100", licenseKey = "TEST-LICENSE-KEY", licensedTo = "Alice Example", email = "alice@example.com",
            purchaseDate = "2024-03-09", downloadURL = "https://www.sketch.com/download",
        )
        val savedItem = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_SOFTWARE_LICENSE, title = "Sketch", softwareLicense = license)),
            "software license not saved",
        )
        assertTrue(savedItem.isSoftwareLicense)
        assertEquals(license, savedItem.softwareLicense)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals(ItemSoftwareLicense(product = "Sketch", version = "100", licensedTo = "Alice Example", email = "alice@example.com"), listed?.softwareLicense)
        assertEquals("Version 100 · Alice Example", listed?.softwareLicense?.summary)
        assertEquals(listOf("Sketch"), model.listTitles { it.isSoftwareLicense })
        assertFalse(model.listTitles { it.isLogin }.contains("Sketch"))
    }

    @Test
    fun testBirthDatePickerTakesOnlyDaysThatExist() {
        assertEquals("1990-04-12", CalendarDate.parse("1990-04-12")?.let(CalendarDate::format))
        assertNotNull(CalendarDate.parse("2024-02-29"))
        assertNull(CalendarDate.parse("2023-02-31"))
        assertNull(CalendarDate.parse("2023-02-29"))
        assertNull(CalendarDate.parse("04/12/1990"))
    }

    @Test
    fun testBirthDateTodayFollowsTheDevicesCalendarDay() {
        val losAngeles = ZoneId.of("America/Los_Angeles")
        val tokyo = ZoneId.of("Asia/Tokyo")
        val evening = Instant.parse("2026-10-15T20:00:00-07:00")
        val morning = Instant.parse("2026-10-16T08:00:00+09:00")

        assertEquals("2026-10-15", CalendarDate.formatToday(evening, losAngeles))
        assertEquals("2026-10-15", CalendarDate.format(CalendarDate.getToday(evening, losAngeles)))
        assertEquals("2026-10-16", CalendarDate.formatToday(morning, tokyo))
        assertEquals("2026-10-16", CalendarDate.format(CalendarDate.getToday(morning, tokyo)))
    }

    @Test
    fun testAPICredentialExpiryWarnsFromTheDayAfterOnTheDevicesCalendar() {
        val losAngeles = ZoneId.of("America/Los_Angeles")
        // October 16 in UTC, still October 15 in Los Angeles.
        val evening = Instant.parse("2026-10-15T20:00:00-07:00")

        assertEquals(ItemAPICredential.EXPIRED_WARNING, ItemAPICredential.describeExpiry("2026-10-14", evening, losAngeles))
        assertEquals("", ItemAPICredential.describeExpiry("2026-10-15", evening, losAngeles))
        assertEquals("", ItemAPICredential.describeExpiry("2031-01-01", evening, losAngeles))
        assertEquals("", ItemAPICredential.describeExpiry("never", evening, losAngeles))
        assertEquals("", ItemAPICredential.describeExpiry("", evening, losAngeles))
        assertEquals("", ItemAPICredential.describeExpiry("2020-02-31", evening, losAngeles))
    }

    @Test
    fun testAPICredentialEditorWarningFollowsTheEditedExpiry() {
        val now = Instant.parse("2026-10-15T12:00:00Z")
        val utc = ZoneId.of("UTC")
        fun describe(expiry: String, saved: ItemAPICredential?) = ItemAPICredential.describeEditedExpiry(expiry, saved, now, utc)
        val expired = ItemAPICredential(expiry = "2020-01-01", expiryWarning = ItemAPICredential.EXPIRED_WARNING)
        val current = ItemAPICredential(expiry = "2031-01-01")

        assertEquals(ItemAPICredential.EXPIRED_WARNING, describe("2020-01-01", expired))
        assertEquals("", describe("2031-01-01", expired), "a future date clears the saved warning")
        assertEquals("", describe("", expired), "removing the expiry clears the saved warning")
        assertEquals(ItemAPICredential.EXPIRED_WARNING, describe("2026-10-14", current), "a past date warns before saving")
        assertEquals(ItemAPICredential.EXPIRED_WARNING, describe("2026-10-14", null), "a new credential warns too")

        val imported = ItemAPICredential(expiry = "2020/01/01", expiryWarning = ItemAPICredential.EXPIRED_WARNING)
        assertEquals(ItemAPICredential.EXPIRED_WARNING, describe("2020/01/01", imported))
        assertEquals("", describe("never", imported))
    }

    @Test
    fun testSaveAddressKeepsOnlyItsFirstStreetLineCityAndCountryInTheList() = runTest {
        val model = createUnlockedModel()
        val address = ItemAddress(street = "1 Main St\nApt 4", city = "Springfield", region = "IL", postalCode = "12345", country = "US")
        val savedItem = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_ADDRESS, title = "Home", address = address)),
            "address not saved",
        )
        assertTrue(savedItem.isAddress)
        assertEquals(address, savedItem.address)
        val listed = model.findListed("personal", savedItem.id)
        assertEquals(ItemAddress(street = "1 Main St", city = "Springfield", country = "US"), listed?.address)
        assertEquals("Springfield, US", listed?.address?.summary)
        assertEquals(listOf("Home"), model.listTitles { it.isAddress })
    }

    @Test
    fun testSaveIdentityKeepsItsAddressInTheListForSearch() = runTest {
        val model = createUnlockedModel()
        val identity = ItemIdentity(firstName = "Alice")
            .withPostalAddress(ItemAddress(street = "1 Main St\nApt 4", city = "Springfield", region = "IL", postalCode = "12345", country = "US"))
        val savedItem = assertNotNull(
            model.itemEditor.saveItem("personal", ItemDetail(category = ItemDetail.CATEGORY_IDENTITY, title = "Alice", identity = identity)),
            "identity not saved",
        )
        val listed = model.findListed("personal", savedItem.id)
        assertEquals(ItemAddress(street = "1 Main St", city = "Springfield", country = "US"), listed?.identity?.postalAddress)
        assertEquals("Alice", listed?.identity?.summary)
    }

    @Test
    fun testIdentityAndCardCarryTheirAddressAndOldOneLineAddress() {
        val json = """{"category":"identity","title":"Alice","identity":{"firstName":"Alice","street":"1 Main St","city":"Springfield","postalCode":"12345","country":"US","address":"1 Main St, Springfield"}}"""
        val item = backendJson.decodeFromString<ItemDetail>(json)
        assertEquals(ItemAddress(street = "1 Main St", city = "Springfield", postalCode = "12345", country = "US"), item.identity?.postalAddress)
        assertEquals("1 Main St, Springfield", item.identity?.address)
        val encoded = backendJson.encodeToString(ItemIdentity.serializer(), assertNotNull(item.identity))
        assertTrue(""""postalCode":"12345"""" in encoded && """"address":"1 Main St, Springfield"""" in encoded, encoded)

        val card = ItemCard().withPostalAddress(ItemAddress(street = "1 Main St", city = "Springfield"))
        assertEquals("1 Main St", card.street)
        assertEquals("Springfield", card.city)
        assertEquals("1 Main St", ItemAddress(street = "1 Main St\nApt 4").summary)
        assertEquals("Address", ItemAddress().summary)
    }

    @Test
    fun testDescribeCardComesFromTheBackend() = runTest {
        val model = createUnlockedModel()
        val description = model.itemEditor.describeCard("4111 1111 1111 1112", "")
        assertEquals("Visa", description?.brand)
        assertEquals("This doesn't look like a valid card number", description?.numberWarning)
    }

    @Test
    fun testGeneratePasswordUsesOptions() = runTest {
        val model = createUnlockedModel()
        val password = model.itemEditor.generatePassword(12, isDigitIncluded = true, isSymbolIncluded = false)
        assertEquals("111111111111", password)
    }

    @Test
    fun testItemDetailDecodesTagsAndDefaultsToNone() {
        val tagged = backendJson.decodeFromString<ItemDetail>("""{"id":"1","title":"GitHub","tags":["work"]}""")
        assertEquals(listOf("work"), tagged.allTag)
        val untagged = backendJson.decodeFromString<ItemDetail>("""{"id":"2","title":"Bank"}""")
        assertEquals(emptyList(), untagged.allTag)
        val encoded = backendJson.encodeToJsonElement(ItemDetail.serializer(), tagged).jsonObject
        assertEquals(listOf("work"), encoded.getValue("tags").jsonArray.map { it.jsonPrimitive.content })
    }
}
