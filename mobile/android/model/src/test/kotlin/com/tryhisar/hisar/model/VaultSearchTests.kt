package com.tryhisar.hisar.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class VaultSearchTests {
    private fun makeReference(
        title: String,
        username: String = "",
        allURL: List<String> = emptyList(),
        category: String = "login",
        vaultName: String = "Personal",
    ) = VaultItemReference(vaultName.lowercase(), vaultName, ItemDetail(id = title.lowercase(), category = category, title = title, username = username, allURL = allURL))

    private fun VaultItemReference.withItem(change: (ItemDetail) -> ItemDetail) = copy(item = change(item))

    private fun List<VaultItemReference>.titles() = map { it.item.title }

    @Test
    fun testFilterMatchesEveryWordAcrossFields() {
        val allItem = listOf(
            makeReference("GitHub", username = "alice", allURL = listOf("https://github.com")),
            makeReference("Netflix", username = "family@example.com", vaultName = "Family"),
            makeReference("Bank", username = "alice"),
        )
        assertEquals(3, VaultLogic.filterAllItem(allItem, searchText = "").size)
        assertEquals(listOf("GitHub", "Bank"), VaultLogic.filterAllItem(allItem, searchText = "ALICE").titles())
        assertEquals(listOf("GitHub"), VaultLogic.filterAllItem(allItem, searchText = "alice github.com").titles())
        assertEquals(listOf("Netflix"), VaultLogic.filterAllItem(allItem, searchText = "family").titles())
        assertTrue(VaultLogic.filterAllItem(allItem, searchText = "nothing").isEmpty())
    }

    @Test
    fun testTagsAreListedWithCountsAndFilterItems() {
        val gitHub = makeReference("GitHub", username = "alice").withItem { it.copy(allTag = listOf("work", "dev")) }
        val bank = makeReference("Bank", vaultName = "Family").withItem { it.copy(allTag = listOf("Finance")) }
        val email = makeReference("Email").withItem { it.copy(allTag = listOf("work")) }
        val allItem = listOf(gitHub, bank, email, makeReference("Untagged"))
        assertEquals(listOf(TagSummary("dev", 1), TagSummary("Finance", 1), TagSummary("work", 2)), VaultLogic.listAllTag(allItem))
        assertEquals(listOf("GitHub", "Email"), VaultLogic.filterAllItemByTag(allItem, tag = "work").titles())
        assertTrue(VaultLogic.filterAllItemByTag(allItem, tag = "nothing").isEmpty())
        assertEquals(listOf("Bank"), VaultLogic.filterAllItem(allItem, searchText = "fin").titles())
        assertTrue(VaultLogic.listAllTag(listOf(makeReference("Untagged"))).isEmpty())
    }

    @Test
    fun testByTypeListsOnlyTypesWithItemsInItsOrder() {
        val allItem = listOf(
            makeReference("Recovery codes", category = ItemDetail.CATEGORY_SECURE_NOTE),
            makeReference("GitHub"),
            makeReference("Bank"),
            makeReference("Email", vaultName = "Family"),
        )
        assertEquals(
            listOf(CategorySummary(ItemCategory.LOGINS, 3), CategorySummary(ItemCategory.SECURE_NOTES, 1)),
            VaultLogic.listAllCategory(allItem),
        )
        assertTrue(VaultLogic.listAllCategory(emptyList()).isEmpty())
    }

    @Test
    fun testSplitTagsDropsBlanksAndRepeats() {
        assertEquals(listOf("work", "dev"), ItemDetail.splitTags(" work, dev ,, work ,"))
        assertEquals(emptyList(), ItemDetail.splitTags("   "))
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

    @Test
    fun testFilterFindsAnIdentityByItsNameAndEmail() {
        val passport = makeReference("Passport", category = ItemDetail.CATEGORY_IDENTITY)
            .withItem { it.copy(identity = ItemIdentity(firstName = "Alice", lastName = "Example")) }
        val work = makeReference("Work", category = ItemDetail.CATEGORY_IDENTITY)
            .withItem { it.copy(identity = ItemIdentity(email = "bob@example.com")) }
        val allItem = listOf(passport, work, makeReference("GitHub"))
        assertEquals(listOf("Passport"), VaultLogic.filterAllItem(allItem, searchText = "alice example").titles())
        assertEquals(listOf("Work"), VaultLogic.filterAllItem(allItem, searchText = "bob@example").titles())
    }

    @Test
    fun testFilterFindsAnIdentityByItsAddress() {
        val identity = ItemIdentity(firstName = "Alice").withPostalAddress(ItemAddress(street = "1 Main St", city = "Springfield", country = "US"))
        val alice = makeReference("Alice", category = ItemDetail.CATEGORY_IDENTITY).withItem { it.copy(identity = identity) }
        val allItem = listOf(alice, makeReference("GitHub"))
        assertEquals(listOf("Alice"), VaultLogic.filterAllItem(allItem, searchText = "springfield").titles())
        assertEquals(listOf("Alice"), VaultLogic.filterAllItem(allItem, searchText = "main st").titles())
    }

    @Test
    fun testFilterFindsAnAPICredentialByItsServiceKeyIDAndHost() {
        val stripe = makeReference("Billing key", category = ItemDetail.CATEGORY_API_CREDENTIAL)
            .withItem { it.copy(apiCredential = ItemAPICredential(service = "Stripe", keyID = "key_live_1", host = "api.stripe.com")) }
        val allItem = listOf(stripe, makeReference("GitHub"))
        assertEquals(listOf("Billing key"), VaultLogic.filterAllItem(allItem, searchText = "stripe").titles())
        assertEquals(listOf("Billing key"), VaultLogic.filterAllItem(allItem, searchText = "api.stripe").titles())
        assertEquals(listOf("Billing key"), VaultLogic.filterAllItem(allItem, searchText = "key_live").titles())
    }

    @Test
    fun testFilterFindsASoftwareLicenseByItsProductVersionAndOwnerButNotItsKey() {
        val license = ItemSoftwareLicense(product = "Sketch", version = "v100", licenseKey = "TEST-LICENSE-KEY", licensedTo = "Alice Example", email = "alice@example.com")
        val sketch = makeReference("Design app", category = ItemDetail.CATEGORY_SOFTWARE_LICENSE).withItem { it.copy(softwareLicense = license) }
        val allItem = listOf(sketch, makeReference("GitHub"))
        for (searchText in listOf("sketch", "v100", "alice example", "alice@")) {
            assertEquals(listOf("Design app"), VaultLogic.filterAllItem(allItem, searchText).titles(), searchText)
        }
        assertTrue(VaultLogic.filterAllItem(allItem, searchText = "test-license").isEmpty())
        assertTrue(ItemCategory.SOFTWARE_LICENSES.includes(sketch.item))
    }

    @Test
    fun testFilterFindsAnAddressByItsStreetCityAndCountry() {
        val home = makeReference("Home", category = ItemDetail.CATEGORY_ADDRESS)
            .withItem { it.copy(address = ItemAddress(street = "1 Main St", city = "Springfield", country = "US")) }
        val allItem = listOf(home, makeReference("GitHub"))
        assertEquals(listOf("Home"), VaultLogic.filterAllItem(allItem, searchText = "springfield").titles())
        assertEquals(listOf("Home"), VaultLogic.filterAllItem(allItem, searchText = "main st").titles())
        assertTrue(ItemCategory.ADDRESSES.includes(home.item))
    }

    @Test
    fun testSortByTitleThenUsername() {
        val allItem = listOf(makeReference("b"), makeReference("A", username = "zed"), makeReference("a", username = "amy"))
        assertEquals(listOf("amy", "zed", ""), VaultLogic.sortAllItem(allItem).map { it.item.username })
    }

    @Test
    fun testServersAreFoundByTheirHostPortAndAddress() {
        val server = makeReference("Build server", username = "admin", allURL = listOf("https://console.example.com"), category = ItemDetail.CATEGORY_SERVER)
            .withItem { it.copy(server = ItemServer(host = "build.example.com", port = "2222")) }
        val ipv6Server = makeReference("Database", category = ItemDetail.CATEGORY_SERVER)
            .withItem { it.copy(server = ItemServer(host = "2001:db8::1", port = "5432")) }
        val note = makeReference("Notes", username = "admin", allURL = listOf("https://notes.example.com"), category = ItemDetail.CATEGORY_SECURE_NOTE)
        val allItem = listOf(server, ipv6Server, note, makeReference("GitHub"))
        for (searchText in listOf("build.example", "2222", "build.example.com:2222")) {
            assertEquals(listOf("Build server"), VaultLogic.filterAllItem(allItem, searchText).titles(), searchText)
        }
        assertEquals(listOf("Database"), VaultLogic.filterAllItem(allItem, searchText = "[2001:db8::1]:5432").titles())
        assertEquals(listOf("Build server", "Database"), allItem.filter { ItemCategory.SERVERS.includes(it.item) }.titles())
        assertEquals(listOf("GitHub"), allItem.filter { ItemCategory.LOGINS.includes(it.item) }.titles())
    }

    @Test
    fun testDatabasesAreFoundByTheirEngineAddressAndName() {
        val database = makeReference("Prod db", username = "app", category = ItemDetail.CATEGORY_DATABASE)
            .withItem { it.copy(database = ItemDatabase(engine = "postgresql", engineName = "PostgreSQL", host = "db.example.com", port = "5432", name = "orders")) }
        val note = makeReference("Notes", category = ItemDetail.CATEGORY_SECURE_NOTE)
        for (searchText in listOf("postgres", "db.example.com:5432/orders", "5432", "orders")) {
            assertEquals(listOf("Prod db"), VaultLogic.filterAllItem(listOf(database, note), searchText).titles(), searchText)
        }
        assertTrue(ItemCategory.DATABASES.includes(database.item))
        assertFalse(ItemCategory.LOGINS.includes(database.item))
    }
}
