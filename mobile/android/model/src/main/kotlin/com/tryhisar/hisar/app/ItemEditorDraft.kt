package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.CardDescription
import com.tryhisar.hisar.model.DatabaseEngine
import com.tryhisar.hisar.model.ItemAddress
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemIdentity
import com.tryhisar.hisar.model.ItemServer
import com.tryhisar.hisar.model.ItemSoftwareLicense
import com.tryhisar.hisar.model.trimmingWhitespaces

/**
 * What the item editor shows: the item being edited, the websites and the tag as typed, and what the card's number last
 * said about it. [original] is the saved item an edit started from, null for a new one.
 */
data class ItemEditorDraft(
    val original: ItemDetail?,
    val item: ItemDetail,
    val allURLText: List<String>,
    val newTagText: String = "",
    /**
     * An identity's or card's one-line address saved before it had address fields stays on screen, even when emptied, until
     * the item is saved.
     */
    val isOldAddressShown: Boolean = false,
    val cardDescription: CardDescription = CardDescription(),
    /** The brand of the number last checked: the type follows it until the user picks a different one. */
    val numberBrand: String = "",
    private val openedItem: ItemDetail = item,
) {
    val isNew: Boolean get() = original == null

    val navigationTitle: String
        get() = when {
            !isNew -> "Edit item"
            item.isSecureNote -> "New secure note"
            item.isCreditCard -> "New credit card"
            item.isIdentity -> "New identity"
            item.isAPICredential -> "New API credential"
            item.isAddress -> "New address"
            item.isSoftwareLicense -> "New software license"
            item.isServer -> "New server"
            item.isDatabase -> "New database"
            else -> "New login"
        }

    val canSave: Boolean get() = item.title.trimmingWhitespaces().isNotEmpty()

    /** Something was typed since the editor opened, so leaving without saving loses it. */
    val hasChanges: Boolean get() = buildItem() != openedItem

    val credentialExpiryWarning: String
        get() = ItemAPICredential.describeEditedExpiry(item.apiCredential?.expiry.orEmpty(), original?.apiCredential)

    /** The types the picker offers: the known brands, plus a type saved from elsewhere that is not one of them. */
    val allCardType: List<String>
        get() {
            val cardType = item.card?.type.orEmpty()
            return if (cardType.isEmpty() || cardType in ItemCard.allBrand) ItemCard.allBrand else ItemCard.allBrand + cardType
        }

    /** A type the user chose that the number does not match; the card still saves. */
    val cardTypeMismatch: String
        get() {
            val cardType = item.card?.type.orEmpty()
            val brand = cardDescription.brand
            return if (cardType.isNotEmpty() && brand.isNotEmpty() && cardType != brand) "The number looks like a $brand card" else ""
        }

    val allCardWarning: List<String>
        get() = listOf(cardDescription.numberWarning, cardTypeMismatch, cardDescription.expiryWarning).filter { it.isNotEmpty() }

    /** The editor's engine choice: a known engine's ID, or "other" for any other engine, whose text is edited below it. */
    val databaseEngineChoice: String
        get() {
            val engine = item.database?.engine.orEmpty()
            return if (DatabaseEngine.isKnown(engine)) engine else OTHER_ENGINE
        }

    fun withItem(transform: (ItemDetail) -> ItemDetail): ItemEditorDraft = copy(item = transform(item))

    /** Writes the card's values, the number and expiry formatted as they are typed, the way Hisar saves them. */
    fun withCard(transform: (ItemCard) -> ItemCard): ItemEditorDraft {
        val previous = item.card ?: ItemCard()
        val card = transform(previous)
        val formatted = card.copy(
            number = if (card.number != previous.number) ItemCard.formatNumberInput(card.number) else card.number,
            expiry = if (card.expiry != previous.expiry) ItemCard.formatExpiryInput(card.expiry) else card.expiry,
        )
        return copy(item = item.copy(card = formatted))
    }

    fun withIdentity(transform: (ItemIdentity) -> ItemIdentity): ItemEditorDraft =
        copy(item = item.copy(identity = transform(item.identity ?: ItemIdentity())))

    fun withAddress(transform: (ItemAddress) -> ItemAddress): ItemEditorDraft =
        copy(item = item.copy(address = transform(item.address ?: ItemAddress())))

    fun withAPICredential(transform: (ItemAPICredential) -> ItemAPICredential): ItemEditorDraft =
        copy(item = item.copy(apiCredential = transform(item.apiCredential ?: ItemAPICredential())))

    fun withServer(transform: (ItemServer) -> ItemServer): ItemEditorDraft =
        copy(item = item.copy(server = transform(item.server ?: ItemServer())))

    fun withDatabase(transform: (ItemDatabase) -> ItemDatabase): ItemEditorDraft =
        copy(item = item.copy(database = transform(item.database ?: ItemDatabase())))

    fun withSoftwareLicense(transform: (ItemSoftwareLicense) -> ItemSoftwareLicense): ItemEditorDraft =
        copy(item = item.copy(softwareLicense = transform(item.softwareLicense ?: ItemSoftwareLicense())))

    /** Picks a known engine, or "other", which empties a known engine so its text can be typed. */
    fun withDatabaseEngineChoice(choice: String): ItemEditorDraft {
        val engine = item.database?.engine.orEmpty()
        return when {
            choice != OTHER_ENGINE -> withDatabase { it.copy(engine = choice) }
            DatabaseEngine.isKnown(engine) -> withDatabase { it.copy(engine = "") }
            else -> this
        }
    }

    /** The address of an identity or a card, or of an address item. */
    val postalAddress: ItemAddress
        get() = when {
            item.isIdentity -> item.identity?.postalAddress ?: ItemAddress()
            item.isCreditCard -> item.card?.postalAddress ?: ItemAddress()
            else -> item.address ?: ItemAddress()
        }

    fun withPostalAddress(address: ItemAddress): ItemEditorDraft = when {
        item.isIdentity -> withIdentity { it.withPostalAddress(address) }
        item.isCreditCard -> withCard { it.withPostalAddress(address) }
        else -> withAddress { address }
    }

    /** The one-line address an identity or a card saved before it had address fields. */
    val oldAddress: String
        get() = when {
            item.isIdentity -> item.identity?.address.orEmpty()
            item.isCreditCard -> item.card?.billingAddress.orEmpty()
            else -> ""
        }

    fun withOldAddress(text: String): ItemEditorDraft = when {
        item.isIdentity -> withIdentity { it.copy(address = text) }
        item.isCreditCard -> withCard { it.copy(billingAddress = text) }
        else -> this
    }

    /** Moves the one-line address into the street, under what the street already holds. */
    fun useOldAddressAsStreet(): ItemEditorDraft {
        val address = postalAddress
        val street = listOf(address.street, oldAddress).filter { it.isNotBlank() }.joinToString("\n")
        return withPostalAddress(address.copy(street = street)).withOldAddress("").copy(isOldAddressShown = false)
    }

    /** Takes what the backend says about the number; the type follows the brand until the user picks another. */
    fun withCardDescription(description: CardDescription): ItemEditorDraft {
        val card = item.card
        val newItem = if (card != null && (card.type.isEmpty() || card.type == numberBrand)) item.copy(card = card.copy(type = description.brand)) else item
        return copy(item = newItem, cardDescription = description, numberBrand = description.brand)
    }

    /** Adds the tags typed so far, separated by commas, without repeats. */
    fun withTagsAdded(): ItemEditorDraft {
        val allNewTag = ItemDetail.splitTags(newTagText).filter { it !in item.allTag }
        return copy(item = item.copy(allTag = item.allTag + allNewTag), newTagText = "")
    }

    /** The item to save: the tags still typed added, and the websites trimmed without blanks. */
    fun buildItem(): ItemDetail {
        val withTags = withTagsAdded().item
        return withTags.copy(allURL = allURLText.map { it.trimmingWhitespaces() }.filter { it.isNotEmpty() })
    }

    companion object {
        const val OTHER_ENGINE = "other"

        /** The editor for [item]: a saved one to edit, or a new item's draft, such as a category or a login AutoFill saw. */
        fun open(item: ItemDetail, isNew: Boolean): ItemEditorDraft {
            var draft = item
            if (draft.isIdentity && draft.identity == null) draft = draft.copy(identity = ItemIdentity())
            if (draft.isAPICredential && draft.apiCredential == null) draft = draft.copy(apiCredential = ItemAPICredential())
            if (draft.isAddress && draft.address == null) draft = draft.copy(address = ItemAddress())
            if (draft.isSoftwareLicense && draft.softwareLicense == null) draft = draft.copy(softwareLicense = ItemSoftwareLicense())
            if (draft.isServer && draft.server == null) draft = draft.copy(server = ItemServer())
            if (draft.isDatabase && draft.database == null) draft = draft.copy(database = ItemDatabase(engine = "postgresql", engineName = "PostgreSQL"))
            if (draft.isCreditCard && draft.card == null) draft = draft.copy(card = ItemCard())
            val card = draft.card
            val opened = ItemEditorDraft(
                original = if (isNew) null else item,
                item = draft,
                allURLText = draft.allURL.ifEmpty { listOf("") },
                isOldAddressShown = draft.identity?.address.orEmpty().isNotEmpty() || card?.billingAddress.orEmpty().isNotEmpty(),
                cardDescription = if (draft.isCreditCard && card != null) {
                    CardDescription(card.brand, card.last4, card.numberWarning, card.expiryWarning)
                } else {
                    CardDescription()
                },
                numberBrand = if (draft.isCreditCard) card?.brand.orEmpty() else "",
            )
            return opened.copy(openedItem = opened.buildItem())
        }
    }
}
