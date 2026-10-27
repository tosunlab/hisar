package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemField
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** What the editor shows and saves as the user types, as the iPhone editor does it. */
class ItemEditorDraftTests {
    private fun openNew(category: String) = ItemEditorDraft.open(ItemDetail(category = category), isNew = true)

    @Test
    fun testCustomFieldNameWarningsPreserveDuplicateValues() {
        val fields = listOf(
            ItemField("Recovery code", "first", true),
            ItemField("recovery CODE", "second", true),
            ItemField("Password", "custom", true),
        )
        val draft = ItemEditorDraft.open(ItemDetail(title = "Login", allField = fields), isNew = true)
        assertEquals("Secret references need a unique name.", draft.fieldNameWarning(0))
        assertEquals("Secret references need a unique name.", draft.fieldNameWarning(1))
        assertEquals("A secret reference with this name reaches the built-in field.", draft.fieldNameWarning(2))
        assertTrue(draft.canSave)
        assertEquals(fields, draft.buildItem().allField)
    }

    @Test
    fun testNewItemsAreTitledByCategoryAndAnEditByItsAction() {
        assertEquals("New login", openNew(ItemDetail.CATEGORY_LOGIN).navigationTitle)
        assertEquals("New secure note", openNew(ItemDetail.CATEGORY_SECURE_NOTE).navigationTitle)
        assertEquals("New credit card", openNew(ItemDetail.CATEGORY_CREDIT_CARD).navigationTitle)
        assertEquals("New identity", openNew(ItemDetail.CATEGORY_IDENTITY).navigationTitle)
        assertEquals("New API credential", openNew(ItemDetail.CATEGORY_API_CREDENTIAL).navigationTitle)
        assertEquals("New address", openNew(ItemDetail.CATEGORY_ADDRESS).navigationTitle)
        assertEquals("New software license", openNew(ItemDetail.CATEGORY_SOFTWARE_LICENSE).navigationTitle)
        assertEquals("New server", openNew(ItemDetail.CATEGORY_SERVER).navigationTitle)
        assertEquals("New database", openNew(ItemDetail.CATEGORY_DATABASE).navigationTitle)
        assertEquals("Edit item", ItemEditorDraft.open(ItemDetail(id = "1", title = "Key", category = ItemDetail.CATEGORY_SSH_KEY), isNew = false).navigationTitle)
    }

    @Test
    fun testANewDatabaseStartsAsPostgreSQLAndOtherKeepsItsTypedEngine() {
        val draft = openNew(ItemDetail.CATEGORY_DATABASE)
        assertEquals("postgresql", draft.databaseEngineChoice)
        val other = draft.withDatabaseEngineChoice(ItemEditorDraft.OTHER_ENGINE)
        assertEquals(ItemEditorDraft.OTHER_ENGINE, other.databaseEngineChoice)
        assertEquals("", other.item.database?.engine)
        val typed = other.withDatabase { it.copy(engine = "db2") }
        assertEquals("db2", typed.withDatabaseEngineChoice(ItemEditorDraft.OTHER_ENGINE).item.database?.engine, "choosing Other again keeps the typed engine")
        assertEquals("mysql", typed.withDatabaseEngineChoice("mysql").item.database?.engine)
    }

    @Test
    fun testCardTypeFollowsTheNumberUntilTheUserPicksAnother() {
        val draft = openNew(ItemDetail.CATEGORY_CREDIT_CARD).withCard { it.copy(number = "4111111111111111") }
        assertEquals("4111 1111 1111 1111", draft.item.card?.number, "the number is grouped as it is typed")
        val visa = draft.withCardDescription(CardDescription(brand = "Visa", last4 = "1111"))
        assertEquals("Visa", visa.item.card?.type)
        val mastercard = visa.withCardDescription(CardDescription(brand = "Mastercard"))
        assertEquals("Mastercard", mastercard.item.card?.type, "a type that followed the number keeps following it")
        val chosen = mastercard.withCard { it.copy(type = "Troy") }.withCardDescription(CardDescription(brand = "Visa"))
        assertEquals("Troy", chosen.item.card?.type)
        assertEquals(listOf("The number looks like a Visa card"), chosen.allCardWarning)
        assertEquals(ItemCard.allBrand, chosen.allCardType)
        val imported = ItemEditorDraft.open(ItemDetail(id = "1", category = ItemDetail.CATEGORY_CREDIT_CARD, card = ItemCard(type = "Elo")), isNew = false)
        assertEquals(ItemCard.allBrand + "Elo", imported.allCardType)
    }

    @Test
    fun testSavingAddsTypedTagsOnceAndDropsBlankWebsites() {
        val draft = ItemEditorDraft.open(ItemDetail(title = "GitHub", allTag = listOf("work")), isNew = true)
            .copy(allURLText = listOf(" https://github.com ", "", "  "), newTagText = "work, code ,, code")
        val item = draft.buildItem()
        assertEquals(listOf("work", "code"), item.allTag)
        assertEquals(listOf("https://github.com"), item.allURL)
    }

    @Test
    fun testOldOneLineAddressMovesUnderTheStreet() {
        val identity = ItemIdentity(street = "1 Main St", address = "Apt 4, Springfield")
        val draft = ItemEditorDraft.open(ItemDetail(id = "1", category = ItemDetail.CATEGORY_IDENTITY, identity = identity), isNew = false)
        assertTrue(draft.isOldAddressShown)
        val moved = draft.useOldAddressAsStreet()
        assertEquals(ItemAddress(street = "1 Main St\nApt 4, Springfield"), moved.postalAddress)
        assertEquals("", moved.oldAddress)
        assertFalse(moved.isOldAddressShown)
        val emptied = draft.withOldAddress("")
        assertTrue(emptied.isOldAddressShown, "an emptied old address stays on screen until the item is saved")
    }

    @Test
    fun testOnlyEditsCountAsUnsavedChanges() {
        val prefilled = ItemEditorDraft.open(ItemDetail(title = "example.com", username = "alice", password = "p", allURL = listOf("https://example.com")), isNew = true)
        assertFalse(prefilled.hasChanges, "AutoFill's prefilled login is not an edit")
        assertTrue(prefilled.withItem { it.copy(notes = "n") }.hasChanges)
        assertTrue(prefilled.copy(newTagText = "work").hasChanges)
        val database = ItemEditorDraft.open(ItemDetail(id = "1", category = ItemDetail.CATEGORY_DATABASE, database = ItemDatabase(engine = "redis")), isNew = false)
        assertFalse(database.hasChanges)
        assertNull(openNew(ItemDetail.CATEGORY_LOGIN).original)
    }
}
