package com.tryhisar.hisar.model

import java.time.Duration
import java.time.Instant
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeParseException
import kotlin.io.encoding.Base64
import kotlin.math.ceil
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.decodeToByteString
import kotlinx.io.bytestring.encode
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.KeepGeneratedSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonTransformingSerializer
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonPrimitive

@Serializable
enum class ThemeAppearanceName {
    @SerialName("system") SYSTEM,
    @SerialName("light") LIGHT,
    @SerialName("dark") DARK,
}

@Serializable
enum class ThemeColorName {
    @SerialName("neutral") NEUTRAL,
    @SerialName("blue") BLUE,
    @SerialName("purple") PURPLE,
    @SerialName("green") GREEN,
}

/** VaultTheme mirrors app.Theme; unknown values fall back to the defaults. */
@Serializable
data class VaultTheme(
    val appearance: ThemeAppearanceName = ThemeAppearanceName.SYSTEM,
    val color: ThemeColorName = ThemeColorName.NEUTRAL,
) {
    /** Whether screens draw dark: System follows the device. */
    fun isDark(isSystemDark: Boolean): Boolean = when (appearance) {
        ThemeAppearanceName.SYSTEM -> isSystemDark
        ThemeAppearanceName.LIGHT -> false
        ThemeAppearanceName.DARK -> true
    }

    companion object {
        val standard = VaultTheme()
    }
}

@Serializable
data class UpdateRequirement(
    val currentVersion: String,
    val minimumVersion: String,
    @SerialName("updateURL") val updateURL: String,
) {
    val currentVersionDescription: String get() = currentVersion.ifEmpty { "a development build" }
}

@OptIn(ExperimentalSerializationApi::class)
@KeepGeneratedSerializer
@Serializable(with = VaultStatusSerializer::class)
data class VaultStatus(
    val state: String = STATE_SIGNED_OUT,
    val server: String = "",
    val email: String = "",
    val name: String = "",
    val biometricAvailable: Boolean = false,
    val biometricEnabled: Boolean = false,
    val biometricReady: Boolean = false,
    val isOffline: Boolean = false,
    val autoLockSeconds: Int = DEFAULT_AUTO_LOCK_SECONDS,
    val signedOutReason: String = "",
    val updateRequired: UpdateRequirement? = null,
) {
    val isSignedIn: Boolean get() = state != STATE_SIGNED_OUT
    val isUnlocked: Boolean get() = state == STATE_UNLOCKED

    companion object {
        const val STATE_SIGNED_OUT = "signed-out"
        const val STATE_LOCKED = "locked"
        const val STATE_UNLOCKED = "unlocked"
        const val DEFAULT_AUTO_LOCK_SECONDS = 300
        val allAutoLockSeconds = listOf(30, 60, 300, 600, 3600)
        val signedOut = VaultStatus()

        fun describeAutoLock(seconds: Int): String = when {
            seconds < 60 -> "$seconds seconds"
            seconds == 60 -> "1 minute"
            seconds == 3600 -> "1 hour"
            seconds % 3600 == 0 -> "${seconds / 3600} hours"
            else -> "${seconds / 60} minutes"
        }

        fun describeShortAutoLock(seconds: Int): String = when {
            seconds < 60 -> "$seconds s"
            seconds == 3600 -> "1 h"
            else -> "${seconds / 60} min"
        }
    }
}

// An auto-lock of zero or less means the backend has none set, which is the default.
@OptIn(ExperimentalSerializationApi::class)
object VaultStatusSerializer : JsonTransformingSerializer<VaultStatus>(VaultStatus.generatedSerializer()) {
    override fun transformDeserialize(element: JsonElement): JsonElement {
        if (element !is JsonObject) return element
        val seconds = element["autoLockSeconds"]?.let { runCatching { it.jsonPrimitive.intOrNull }.getOrNull() }
        return if (seconds != null && seconds > 0) element else JsonObject(element - "autoLockSeconds")
    }
}

@Serializable
data class ReadIssue(val vaultId: String, val itemId: String = "", val revision: String = "", val kind: String, val canRename: Boolean = false, val canDelete: Boolean = false)

@Serializable
data class VaultSummary(
    val id: String,
    val name: String = "",
    val type: String = "",
    val role: String = "",
    val itemCount: Int = 0,
    /** Set for organization vaults only. */
    @SerialName("organizationId") val organizationID: String = "",
    val readIssues: List<ReadIssue> = emptyList(),
    val isPersonalExtra: Boolean = false,
) {
    val isOrganization: Boolean get() = type == TYPE_ORGANIZATION

    companion object {
        const val TYPE_PERSONAL = "personal"
        const val TYPE_SHARED = "shared"
        const val TYPE_ORGANIZATION = "organization"
    }
}

@Serializable
data class ItemField(val name: String, val value: String, val concealed: Boolean)

@Serializable
data class ItemDetail(
    val id: String = "",
    val revision: String = "",
    val readIssue: ReadIssue? = null,
    val category: String = CATEGORY_LOGIN,
    val title: String = "",
    val username: String = "",
    @SerialName("urls") val allURL: List<String> = emptyList(),
    @SerialName("tags") val allTag: List<String> = emptyList(),
    val password: String = "",
    val notes: String = "",
    @SerialName("fields") val allField: List<ItemField> = emptyList(),
    val publicKey: String = "",
    val fingerprint: String = "",
    val keyType: String = "",
    val oneTimePassword: String = "",
    val hasOneTimePassword: Boolean = false,
    @SerialName("passkeys") val allPasskey: List<ItemPasskey> = emptyList(),
    /** A login with nothing but notes and custom fields, which the backend can turn into a secure note. */
    val canConvertToSecureNote: Boolean = false,
    /**
     * A credit card's values, stored as named custom fields; allField then holds only the others. Summaries keep only the
     * brand and last four digits.
     */
    val card: ItemCard? = null,
    /** An identity's values, stored as named custom fields like a card's. Summaries keep only the name and email. */
    val identity: ItemIdentity? = null,
    /** The user starred the item. Favorites belong to the user's account, so people sharing the vault don't see them. */
    val isFavorite: Boolean = false,
    /** An API credential's values; summaries keep only the service and host. */
    val apiCredential: ItemAPICredential? = null,
    /** An address's values; summaries keep only the first line of the street, the city and the country. */
    val address: ItemAddress? = null,
    /** A software license's values; summaries keep only the product, version, licensed-to name and email. */
    val softwareLicense: ItemSoftwareLicense? = null,
    /**
     * A server's host, port and linked SSH key; its username, password and admin console are the item's own. Summaries
     * keep only the host and port.
     */
    val server: ItemServer? = null,
    /**
     * A database's engine, host, port, name and options; its username and password are the item's own. The backend builds
     * its connection string. Summaries keep only the engine, host, port and name.
     */
    val database: ItemDatabase? = null,
    /**
     * The icon the user chose, or null for the automatic one: a login's website icon, else the category's icon or the
     * initial.
     */
    val icon: ChosenIcon? = null,
    /** The Android apps a login fills into, which the user linked on an Android phone; saving the item keeps them. */
    @SerialName("androidApps") val allAndroidApp: List<ItemAndroidApp> = emptyList(),
) {
    val isLogin: Boolean get() = category.isEmpty() || category == CATEGORY_LOGIN
    val isSSHKey: Boolean get() = category == CATEGORY_SSH_KEY
    val isSecureNote: Boolean get() = category == CATEGORY_SECURE_NOTE
    val isCreditCard: Boolean get() = category == CATEGORY_CREDIT_CARD
    val isIdentity: Boolean get() = category == CATEGORY_IDENTITY
    val isAPICredential: Boolean get() = category == CATEGORY_API_CREDENTIAL
    val isAddress: Boolean get() = category == CATEGORY_ADDRESS
    val isSoftwareLicense: Boolean get() = category == CATEGORY_SOFTWARE_LICENSE
    val isServer: Boolean get() = category == CATEGORY_SERVER
    val isDatabase: Boolean get() = category == CATEGORY_DATABASE

    /** AutoFill offers the item's username and password on its websites: a login's, and a server's admin console. */
    val offersLoginFill: Boolean get() = isLogin || isServer

    /** The website whose icon a login shows when it has no icon of its own: its first https URL. */
    val websiteIconURL: String?
        get() {
            if (!isLogin || icon != null) return null
            return allURL.map { it.trimmingWhitespaces() }.firstOrNull { it.lowercase().startsWith("https://") }
        }

    companion object {
        const val CATEGORY_LOGIN = "login"
        const val CATEGORY_SSH_KEY = "ssh-key"
        const val CATEGORY_SECURE_NOTE = "secure-note"
        const val CATEGORY_CREDIT_CARD = "credit-card"
        const val CATEGORY_IDENTITY = "identity"
        const val CATEGORY_API_CREDENTIAL = "api-credential"
        const val CATEGORY_ADDRESS = "address"
        const val CATEGORY_SOFTWARE_LICENSE = "software-license"
        const val CATEGORY_SERVER = "server"
        const val CATEGORY_DATABASE = "database"

        /** The tags typed into one field, separated by commas, without blanks or repeats. */
        fun splitTags(text: String): List<String> =
            text.split(",").map { it.trimmingWhitespacesAndNewlines() }.filter { it.isNotEmpty() }.distinct()
    }
}

/**
 * The names core/item stores an address's values under; identities and cards keep their address under the same names, and
 * copying and secret references use them.
 */
object AddressFieldName {
    const val STREET = "Street"
    const val CITY = "City"
    const val REGION = "Region"
    const val POSTAL_CODE = "Postal code"
    const val COUNTRY = "Country"
}

/** A postal address; `street` may hold several lines. Identities and cards keep one in the same keys. */
@Serializable
data class ItemAddress(
    val street: String = "",
    val city: String = "",
    val region: String = "",
    val postalCode: String = "",
    val country: String = "",
) {
    /** The address's line in lists: its city and country, or the first line of its street, such as "Springfield, US". */
    val summary: String
        get() {
            val cityAndCountry = listOf(city, country).filter { it.isNotEmpty() }.joinToString(", ")
            if (cityAndCountry.isNotEmpty()) return cityAndCountry
            return street.split("\n", limit = 2).first().ifEmpty { "Address" }
        }

    /** The values as named rows, in the order core/item stores them. */
    val allNamedValue: List<Pair<String, String>>
        get() = listOf(
            AddressFieldName.STREET to street,
            AddressFieldName.CITY to city,
            AddressFieldName.REGION to region,
            AddressFieldName.POSTAL_CODE to postalCode,
            AddressFieldName.COUNTRY to country,
        )
}

/** The names core/item stores an identity's values under; copying and secret references use them. */
object IdentityFieldName {
    const val FIRST_NAME = "First name"
    const val MIDDLE_NAME = "Middle name"
    const val LAST_NAME = "Last name"
    const val BIRTH_DATE = "Birth date"
    const val EMAIL = "Email"
    const val PHONE = "Phone"
    /** The one-line address an identity had before it kept one in the address fields. */
    const val ADDRESS = "Address"
    const val PASSPORT_NUMBER = "Passport number"
    const val DRIVERS_LICENSE_NUMBER = "Driver's license number"
    const val NATIONAL_ID_NUMBER = "National ID number"
}

/**
 * A person's details; the document numbers are optional and concealed. The address is in `street` to `country`;
 * `address` is a one-line address saved before identities had those, kept as it is.
 */
@Serializable
data class ItemIdentity(
    val firstName: String = "",
    val middleName: String = "",
    val lastName: String = "",
    val birthDate: String = "",
    val email: String = "",
    val phone: String = "",
    val street: String = "",
    val city: String = "",
    val region: String = "",
    val postalCode: String = "",
    val country: String = "",
    val address: String = "",
    val passportNumber: String = "",
    val driversLicenseNumber: String = "",
    @SerialName("nationalIdNumber") val nationalIDNumber: String = "",
) {
    /** The identity's address, for the editor and detail view an address item uses. */
    val postalAddress: ItemAddress get() = ItemAddress(street, city, region, postalCode, country)

    fun withPostalAddress(address: ItemAddress): ItemIdentity =
        copy(street = address.street, city = address.city, region = address.region, postalCode = address.postalCode, country = address.country)

    /** The name parts that are set, such as "Alice M. Example". */
    val fullName: String get() = listOf(firstName, middleName, lastName).filter { it.isNotEmpty() }.joinToString(" ")

    /** The identity's line in lists: the full name, or the email when it has no name. */
    val summary: String get() = fullName.ifEmpty { email.ifEmpty { "Identity" } }
}

/** A day without a time or time zone, stored as YYYY-MM-DD: an identity's birth date or an API credential's expiry. */
object CalendarDate {
    private val storedForm = Regex("""\d{4}-\d{2}-\d{2}""")

    /** The day a stored YYYY-MM-DD date names, or null for any other text and for a day that does not exist. */
    fun parse(text: String): LocalDate? {
        if (!storedForm.matches(text)) return null
        return try {
            LocalDate.parse(text)
        } catch (_: DateTimeParseException) {
            null
        }
    }

    /** The day as it is stored, YYYY-MM-DD. */
    fun format(date: LocalDate): String = date.toString()

    /** Today in the device's time zone, as stored. */
    fun formatToday(now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): String = format(getToday(now, zone))

    /** Today in the device's time zone, such as the latest birth date a picker offers. */
    fun getToday(now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): LocalDate = LocalDate.ofInstant(now, zone)
}

/** The names core/item stores an API credential's values under; copying and hisar:// references use them. */
object APICredentialFieldName {
    const val SERVICE = "Service"
    const val KEY_ID = "Key ID"
    const val SECRET = "Secret"
    const val HOST = "Host"
    const val EXPIRY = "Expiry"
}

/** An API key; the secret is concealed. `expiryWarning` comes from the backend once the expiry has passed. */
@Serializable
data class ItemAPICredential(
    val service: String = "",
    @SerialName("keyId") val keyID: String = "",
    val secret: String = "",
    val host: String = "",
    val expiry: String = "",
    val expiryWarning: String = "",
) {
    /** The credential's line in lists: its service and host, such as "Stripe · api.stripe.com". */
    val summary: String
        get() = listOf(service, host).filter { it.isNotEmpty() }.joinToString(" · ").ifEmpty { "API credential" }

    companion object {
        const val EXPIRED_WARNING = "This credential has expired"

        /**
         * The warning for a YYYY-MM-DD expiry once its day has passed in the device's time zone, as core/item
         * DescribeAPICredentialExpiry gives it; "" for any other value.
         */
        fun describeExpiry(expiry: String, now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): String {
            if (CalendarDate.parse(expiry) == null) return ""
            // YYYY-MM-DD dates sort as text.
            return if (expiry < CalendarDate.formatToday(now, zone)) EXPIRED_WARNING else ""
        }

        /**
         * The editor's warning, which follows the expiry as it is edited rather than the one the backend gave for the saved
         * value. A saved value in another form that the backend still reads as a date, such as 2020/01/01, keeps the
         * backend's warning until it changes.
         */
        fun describeEditedExpiry(
            expiry: String,
            saved: ItemAPICredential?,
            now: Instant = Instant.now(),
            zone: ZoneId = ZoneId.systemDefault(),
        ): String {
            if (saved != null && CalendarDate.parse(expiry) == null && expiry == saved.expiry) return saved.expiryWarning
            return describeExpiry(expiry, now, zone)
        }
    }
}

/** The names core/item stores a server's values under; its username, password and admin console are the item's own. */
object ServerFieldName {
    const val HOST = "Host"
    const val PORT = "Port"
    const val SSH_KEY = "SSH key"
}

/**
 * A server's values besides its login data. `sshKeyID` names an SSH key item in the same vault; the backend fills in its
 * title and fingerprint, which are empty when the key is gone.
 */
@Serializable
data class ItemServer(
    val host: String = "",
    val port: String = "",
    @SerialName("sshKeyId") val sshKeyID: String = "",
    val sshKeyTitle: String = "",
    val sshKeyFingerprint: String = "",
    val portWarning: String = "",
) {
    /** The host and port, such as "build.example.com:2222", with brackets around an IPv6 host. */
    val address: String
        get() {
            if (host.isEmpty() || port.isEmpty()) return host
            return if (":" in host) "[$host]:$port" else "$host:$port"
        }

    /** The server's line in lists: its address and username, as the Mac app shows it. */
    fun summary(username: String): String =
        listOf(address, username).filter { it.isNotEmpty() }.joinToString(" · ").ifEmpty { "Server" }

    companion object {
        const val PORT_WARNING_TEXT = "The port is not a number from 1 to 65535"

        /** The editor's warning for a port that is not a number from 1 to 65535, as core/item gives it; "" for an empty port. */
        fun describePort(port: String): String {
            val trimmedPort = port.trimmingWhitespaces()
            if (trimmedPort.isEmpty()) return ""
            val number = if (trimmedPort.all { it.isAsciiDigit() }) trimmedPort.toIntOrNull() else null
            return if (number != null && number in 1..65535) "" else PORT_WARNING_TEXT
        }
    }
}

/**
 * The names core/item stores a database's values under; its username and password are the item's own. "connection string"
 * names the connection string the backend builds, for copying and hisar:// references.
 */
object DatabaseFieldName {
    const val ENGINE = "Engine"
    const val HOST = "Host"
    const val PORT = "Port"
    const val NAME = "Database"
    const val OPTIONS = "Options"
    const val CONNECTION_STRING = "connection string"
}

/** A database engine the editor offers and Hisar builds connection strings for, as core/item lists them. */
data class DatabaseEngine(val id: String, val name: String) {
    companion object {
        val allKnown = listOf(
            DatabaseEngine("postgresql", "PostgreSQL"),
            DatabaseEngine("mysql", "MySQL"),
            DatabaseEngine("mariadb", "MariaDB"),
            DatabaseEngine("sqlserver", "SQL Server"),
            DatabaseEngine("oracle", "Oracle"),
            DatabaseEngine("mongodb", "MongoDB"),
            DatabaseEngine("redis", "Redis"),
        )

        fun isKnown(engine: String): Boolean = allKnown.any { it.id == engine }
    }
}

/**
 * A database's values besides its login data. `engine` is a known engine's ID or any other engine as typed, which
 * `engineName` names; `connectionString` is "" for another engine or without a host. The backend reads only engine, host,
 * port, name and options.
 */
@Serializable
data class ItemDatabase(
    val engine: String = "",
    val engineName: String = "",
    val host: String = "",
    val port: String = "",
    val name: String = "",
    val options: String = "",
    val connectionString: String = "",
    val portWarning: String = "",
) {
    /** The host and port, such as "db.example.com:5432", with brackets around an IPv6 host, as a server's. */
    val address: String get() = ItemServer(host = host, port = port).address

    /** The address and database name, such as "db.example.com:5432/app". */
    val location: String
        get() {
            if (name.isEmpty()) return address
            return if (address.isEmpty()) name else "$address/$name"
        }

    /** The database's line in lists: its engine, address and name, as the Mac app shows it. */
    val summary: String
        get() = listOf(engineName, location).filter { it.isNotEmpty() }.joinToString(" · ").ifEmpty { "Database" }
}

/** The names core/item stores a software license's values under; copying and hisar:// references use them. */
object SoftwareLicenseFieldName {
    const val PRODUCT = "Product"
    const val VERSION = "Version"
    const val LICENSE_KEY = "License key"
    const val LICENSED_TO = "Licensed to"
    const val EMAIL = "Email"
    const val PURCHASE_DATE = "Purchase date"
    const val DOWNLOAD_URL = "Download URL"
}

/**
 * A software license; the license key is concealed. The download URL is a field, not the item's website, so AutoFill
 * never offers the license on that site.
 */
@Serializable
data class ItemSoftwareLicense(
    val product: String = "",
    val version: String = "",
    val licenseKey: String = "",
    val licensedTo: String = "",
    val email: String = "",
    val purchaseDate: String = "",
    @SerialName("downloadUrl") val downloadURL: String = "",
) {
    /**
     * The license's line in lists: its version and who it is licensed to, such as "Version 100 · Alice Example", or its
     * email or product when it has neither, as the Mac app shows it.
     */
    val summary: String
        get() {
            val versionText = if (version.firstOrNull()?.isNumber() == true) "Version $version" else version
            val parts = listOf(versionText, licensedTo.ifEmpty { email }).filter { it.isNotEmpty() }
            if (parts.isNotEmpty()) return parts.joinToString(" · ")
            return product.ifEmpty { "Software license" }
        }
}

/** One of the user's starred items with the vault it is in; the item carries no secrets, as in a vault's list. */
@Serializable
data class FavoriteItem(
    @SerialName("vaultId") val vaultID: String,
    val vaultName: String,
    val vaultType: String,
    val item: ItemDetail,
) {
    val id: String get() = LoginMatch.buildRecordIdentifier(vaultID, item.id)
}

/** The names core/item stores a card's values under; copying and secret references use them. */
object CardFieldName {
    const val CARDHOLDER = "Cardholder"
    const val TYPE = "Type"
    const val NUMBER = "Card number"
    const val EXPIRY = "Expiry"
    const val SECURITY_CODE = "Security code"
    const val PIN = "PIN"
    /** The one-line address a card had before it kept one in the address fields. */
    const val BILLING_ADDRESS = "Billing address"
}

/** A card's values. `type` is the brand saved with the card, which the user may choose; `brand` is the one its number points to. */
@Serializable
data class ItemCard(
    val cardholder: String = "",
    val type: String = "",
    val number: String = "",
    val expiry: String = "",
    val securityCode: String = "",
    val pin: String = "",
    val street: String = "",
    val city: String = "",
    val region: String = "",
    val postalCode: String = "",
    val country: String = "",
    /** A one-line address saved before cards kept one in `street` to `country`, kept as it is. */
    val billingAddress: String = "",
    val brand: String = "",
    val last4: String = "",
    val numberWarning: String = "",
    val expiryWarning: String = "",
) {
    /** The card's billing address, for the editor and detail view an address item uses. */
    val postalAddress: ItemAddress get() = ItemAddress(street, city, region, postalCode, country)

    fun withPostalAddress(address: ItemAddress): ItemCard =
        copy(street = address.street, city = address.city, region = address.region, postalCode = address.postalCode, country = address.country)

    /** The card's line in lists: its type (or the brand its number points to) and last four digits, such as "Visa •••• 4242". */
    val summary: String
        get() = listOf(type.ifEmpty { brand }, if (last4.isEmpty()) "" else "•••• $last4").filter { it.isNotEmpty() }
            .joinToString(" ").ifEmpty { "Credit card" }

    companion object {
        /** The brands an editor offers as a card's type, in the order of core/item ListCardBrands. */
        val allBrand = listOf("Visa", "Mastercard", "American Express", "Discover", "Diners Club", "JCB", "UnionPay", "Maestro", "Troy")

        private val dinersClubPrefix = Regex("^3(0[0-5]|[689])")
        private val yearFirstExpiry = Regex("""(\d{4})\s*[/.\-]\s*(\d{1,2})""")
        private val monthFirstExpiry = Regex("""(\d{1,2})\s*[/.\-]\s*(\d*)""")
        private val singleDigitMonth = Regex("^([2-9]|1[3-9])")

        /**
         * Groups a card number as it is typed, the way core/item saves it: 4-6-5 for American Express, 4-6-4 for a 14-digit
         * Diners Club number and fours otherwise. Text with other characters is left as typed.
         */
        fun formatNumberInput(value: String): String {
            if (!value.all { it.isAsciiDigit() || it == ' ' || it == '-' }) return value
            val digits = value.filter { it.isAsciiDigit() }
            val allGroupSize = when {
                (digits.startsWith("34") || digits.startsWith("37")) && digits.length <= 15 -> listOf(4, 6, 5)
                dinersClubPrefix.containsMatchIn(digits) && digits.length <= 14 -> listOf(4, 6, 4)
                else -> listOf(4)
            }
            val allGroup = mutableListOf<String>()
            var start = 0
            while (start < digits.length) {
                val size = allGroupSize[minOf(allGroup.size, allGroupSize.size - 1)]
                allGroup += digits.substring(start, minOf(start + size, digits.length))
                start += size
            }
            return allGroup.joinToString(" ")
        }

        /**
         * Writes an expiry as MM/YY while it is typed: "1230" becomes "12/30", "3" becomes "03" and "1/" becomes "01/". A
         * pasted 12/2030 or 2030-12 becomes 12/30, and text with other characters is left as typed.
         */
        fun formatExpiryInput(value: String): String {
            if (!value.all { it.isAsciiDigit() || it in " /.-" }) return value
            val trimmed = value.trimmingWhitespaces()
            yearFirstExpiry.matchEntire(trimmed)?.let { match ->
                val (year, month) = match.destructured
                return "${pad(month)}/${year.takeLast(2)}"
            }
            monthFirstExpiry.matchEntire(trimmed)?.let { match ->
                val (month, year) = match.destructured
                return "${pad(month)}/${if (year.length == 4) year.takeLast(2) else year.take(2)}"
            }
            var digits = value.filter { it.isAsciiDigit() }
            if (singleDigitMonth.containsMatchIn(digits)) digits = "0$digits"
            if (digits.length <= 2) return digits
            val year = if (digits.length == 6) digits.takeLast(2) else digits.drop(2).take(2)
            return "${digits.take(2)}/$year"
        }

        private fun pad(month: String): String = if (month.length == 1) "0$month" else month
    }
}

@Serializable
data class CardDescription(
    val brand: String = "",
    val last4: String = "",
    val numberWarning: String = "",
    val expiryWarning: String = "",
)

@Serializable
data class SignUpResult(
    val status: VaultStatus,
    val secretKey: String = "",
    val pendingOperationId: String = "",
    val setupState: String = "",
    val problems: List<String> = emptyList(),
)

@Serializable
data class PendingRecovery(
    val operationId: String,
    val kind: String,
    val origin: String,
    val accountId: String,
    val email: String,
    val name: String,
    val phase: String,
    val kitSaved: Boolean = false,
    val problems: List<String> = emptyList(),
    val manualUnlock: Boolean = false,
    val keychainProtected: Boolean = false,
)

val PendingRecovery.reminderLabel: String
    get() = when {
        phase == "manual-unlock" -> "Sign in with your saved Emergency Kit."
        phase == "awaiting-kit" || phase == "delivery-pending" ->
            if (kind == "signup") "Save your Emergency Kit to finish setting up $email."
            else "Save your new Emergency Kit to finish replacing your Secret Key."
        else -> "Finish setting up $email."
    }

val PendingRecovery.reminderHint: String
    get() = when (phase) {
        "manual-unlock" -> "Opens sign-in with your saved Emergency Kit"
        "awaiting-kit", "delivery-pending" -> "Opens the Emergency Kit step"
        else -> "Opens the unfinished account setup"
    }

@Serializable
data class PendingRecoveryExport(val secretKey: String, val path: String)

@Serializable
data class LoginMatch(
    val vaultID: String,
    val itemID: String,
    val title: String = "",
    val username: String = "",
    val vaultName: String = "",
    val hasOneTimePassword: Boolean = false,
    val isPersonal: Boolean = false,
) {
    val id: String get() = buildRecordIdentifier(vaultID, itemID)

    companion object {
        fun buildRecordIdentifier(vaultID: String, itemID: String): String = "$vaultID/$itemID"

        /** The vault and item IDs of a record identifier, or null when it names no item. */
        fun parseRecordIdentifier(recordIdentifier: String): Pair<String, String>? {
            val allPart = recordIdentifier.split("/", limit = 2)
            if (allPart.size != 2 || allPart[0].isEmpty() || allPart[1].isEmpty()) return null
            return allPart[0] to allPart[1]
        }
    }
}

@Serializable
data class LoginCredential(val username: String = "", val password: String = "", val hasOneTimePassword: Boolean = false)

@Serializable
data class OneTimeCode(val code: String, val expiresAt: String, val period: Int) {
    val expiryDate: Instant? get() = parseTimestamp(expiresAt)

    val formattedCode: String
        get() {
            val middle = (code.length + 1) / 2
            return "${code.substring(0, middle)} ${code.substring(middle)}".trimmingWhitespaces()
        }

    fun countSecondsLeft(date: Instant): Int {
        val expiryDate = expiryDate ?: return 0
        return maxOf(0, ceil(Duration.between(date, expiryDate).toMillis() / 1000.0).toInt())
    }
}

/** An RFC 3339 time from the backend, with or without fractional seconds, or null for any other text. */
fun parseTimestamp(text: String): Instant? = try {
    OffsetDateTime.parse(text).toInstant()
} catch (_: DateTimeParseException) {
    null
}

object Base64URL {
    private val urlSafe = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT_OPTIONAL)

    fun encode(data: ByteString): String = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT).encode(data)

    fun decode(text: String): ByteString? = try {
        urlSafe.decodeToByteString(text)
    } catch (_: IllegalArgumentException) {
        null
    }
}

@Serializable
data class ItemPasskey(
    @SerialName("credentialId") val credentialID: String,
    @SerialName("relyingPartyId") val relyingPartyID: String = "",
    val userName: String = "",
    val userDisplayName: String = "",
    val createdAt: String = "",
) {
    val id: String get() = credentialID
    val createdDate: Instant? get() = parseTimestamp(createdAt)
}

/** An Android app a login fills into: its package name and the SHA-256 of its signing certificate, in colon hex. */
@Serializable
data class ItemAndroidApp(
    val packageName: String,
    @SerialName("certificateSha256") val certificateSHA256: String,
)

@Serializable
data class StoredPasskey(
    @SerialName("credentialId") val credentialID: String,
    @SerialName("relyingPartyId") val relyingPartyID: String = "",
    val userName: String = "",
    val userDisplayName: String = "",
    val userHandle: String = "",
    val vaultID: String,
    val itemID: String,
    val title: String = "",
) {
    val id: String get() = credentialID
    val recordIdentifier: String get() = LoginMatch.buildRecordIdentifier(vaultID, itemID)
}

@Serializable
data class PasskeyRegistrationOptions(
    val isExcluded: Boolean = false,
    @SerialName("logins") val allLogin: List<LoginMatch> = emptyList(),
)

/** [isUserVerificationDiscouraged] when the site's `userVerification` is `discouraged`; Hisar then signs without asking the device owner. */
data class PasskeyAssertionRequest(
    val relyingPartyID: String,
    val clientDataHash: ByteString,
    val allAllowedCredentialID: List<ByteString> = emptyList(),
    val userVerification: String = "preferred",
) {
    val isUserVerificationDiscouraged: Boolean get() = userVerification == "discouraged"
}

data class PasskeyRegistrationRequest(
    val relyingPartyID: String,
    val userName: String,
    val userHandle: ByteString,
    val clientDataHash: ByteString,
    val allSupportedAlgorithm: List<Int> = emptyList(),
    val allExcludedCredentialID: List<ByteString> = emptyList(),
    val userVerification: String = "preferred",
) {
    val isUserVerificationDiscouraged: Boolean get() = userVerification == "discouraged"
    val isAlgorithmSupported: Boolean get() = isAlgorithmSupported(allSupportedAlgorithm)

    companion object {
        const val ALGORITHM_ES256 = -7

        /** Hisar's passkeys are ES256; an empty list leaves the choice to the authenticator. */
        fun isAlgorithmSupported(allAlgorithm: List<Int>): Boolean = allAlgorithm.isEmpty() || ALGORITHM_ES256 in allAlgorithm
    }
}

@Serializable
data class PasskeyRegistrationResult(
    @SerialName("credentialId") val credentialID: String,
    val attestationObject: String,
    val authenticatorData: String,
    val vaultID: String,
    val itemID: String,
)

@Serializable
data class PasskeyAssertionResult(
    @SerialName("credentialId") val credentialID: String,
    val authenticatorData: String,
    val signature: String,
    val userHandle: String,
)

data class PasskeyAssertionCredential(
    val relyingPartyID: String,
    val clientDataHash: ByteString,
    val credentialID: ByteString,
    val authenticatorData: ByteString,
    val signature: ByteString,
    val userHandle: ByteString,
) {
    companion object {
        /** The credential the system gets, or null when the backend's answer is not base64url. */
        fun create(relyingPartyID: String, clientDataHash: ByteString, result: PasskeyAssertionResult): PasskeyAssertionCredential? {
            return PasskeyAssertionCredential(
                relyingPartyID = relyingPartyID,
                clientDataHash = clientDataHash,
                credentialID = Base64URL.decode(result.credentialID) ?: return null,
                authenticatorData = Base64URL.decode(result.authenticatorData) ?: return null,
                signature = Base64URL.decode(result.signature) ?: return null,
                userHandle = Base64URL.decode(result.userHandle) ?: return null,
            )
        }
    }
}

data class PasskeyRegistrationCredential(
    val relyingPartyID: String,
    val clientDataHash: ByteString,
    val credentialID: ByteString,
    val attestationObject: ByteString,
    val authenticatorData: ByteString,
) {
    companion object {
        /** The credential the system gets, or null when the backend's answer is not base64url. */
        fun create(relyingPartyID: String, clientDataHash: ByteString, result: PasskeyRegistrationResult): PasskeyRegistrationCredential? {
            return PasskeyRegistrationCredential(
                relyingPartyID = relyingPartyID,
                clientDataHash = clientDataHash,
                credentialID = Base64URL.decode(result.credentialID) ?: return null,
                attestationObject = Base64URL.decode(result.attestationObject) ?: return null,
                authenticatorData = Base64URL.decode(result.authenticatorData) ?: return null,
            )
        }
    }
}

/** Client data Hisar wrote for a caller that sends only a challenge, and the SHA-256 that the signature covers. */
data class PasskeyClientData(val json: ByteString, val hash: ByteString)

/**
 * A person you share a vault with (role admin or member on the server), someone with access to a vault (role owner, editor
 * or reader) or a member of an organization (role admin or member of the organization).
 */
@Serializable
data class Member(
    @SerialName("accountId") val accountID: String = "",
    val name: String = "",
    val email: String = "",
    val role: String = "",
    val securityCode: String = "",
    val isCurrentUser: Boolean = false,
    val isKeyChanged: Boolean = false,
) {
    val id: String get() = accountID

    companion object {
        const val ROLE_ADMIN = "admin"
        const val ROLE_MEMBER = "member"
        const val VAULT_ROLE_OWNER = "owner"
        const val VAULT_ROLE_EDITOR = "editor"
        const val VAULT_ROLE_READER = "reader"
        val allVaultRole = listOf(VAULT_ROLE_OWNER, VAULT_ROLE_EDITOR, VAULT_ROLE_READER)
    }
}

@Serializable
data class ReceivedShareInvitation(
    val id: String,
    @SerialName("vaultId") val vaultID: String,
    val role: String,
    val invitedByName: String,
    val invitedByEmail: String,
    val expiresAt: String,
) {
    val inviterName: String get() = invitedByName.ifEmpty { invitedByEmail }
}

@Serializable
data class SentShareInvitation(
    val id: String,
    @SerialName("vaultId") val vaultID: String,
    val vaultName: String,
    val email: String,
    val role: String,
    val state: String,
    val expiresAt: String,
    val requiresConfirmation: Boolean = false,
    @SerialName("recipientId") val recipientID: String = "",
    val securityCode: String = "",
    val reviewToken: String = "",
    val recipientName: String = "",
) {
    companion object {
        const val STATE_ACCEPTED = "accepted"
    }
}

/** A share this user accepted that the owner's app has not finished yet. */
@Serializable
data class WaitingShareInvitation(
    val id: String,
    @SerialName("vaultId") val vaultID: String,
    val role: String,
    val invitedByName: String,
    val invitedByEmail: String,
    val acceptedAt: String,
) {
    val inviterName: String
        get() = if (invitedByName.isEmpty() || invitedByName == invitedByEmail) invitedByEmail else invitedByName

    fun describe(now: Instant): String {
        val since = formatTimeAgo(acceptedAt, now)
        return "Waiting for $inviterName's Hisar to finish sharing" + if (since.isEmpty()) "" else " · accepted $since"
    }
}

/** A share someone accepted that this locked app finishes once unlocked; the vault name is encrypted until then. */
@Serializable
data class ShareAwaitingUnlock(
    @SerialName("invitationId") val invitationID: String,
    val recipientName: String,
    val recipientEmail: String,
    val acceptedAt: String,
) {
    val id: String get() = invitationID
    val recipientDisplayName: String
        get() = if (recipientName.isEmpty() || recipientName == recipientEmail) recipientEmail else recipientName
    val message: String get() = "$recipientDisplayName accepted your invitation to a vault — unlock to finish sharing"
}

/** A short "since" for banners: "just now", "5 min ago", "2 h ago", "yesterday", "3 days ago". */
fun formatTimeAgo(timestamp: String, now: Instant): String {
    val date = parseTimestamp(timestamp) ?: return ""
    val minuteCount = (Duration.between(date, now).toMillis() / 60_000.0).toInt()
    if (minuteCount < 1) return "just now"
    if (minuteCount < 60) return "$minuteCount min ago"
    val hourCount = minuteCount / 60
    if (hourCount < 24) return "$hourCount h ago"
    val dayCount = hourCount / 24
    return if (dayCount == 1) "yesterday" else "$dayCount days ago"
}

@Serializable
data class ShareInvitations(
    @SerialName("received") val allReceived: List<ReceivedShareInvitation> = emptyList(),
    @SerialName("sent") val allSent: List<SentShareInvitation> = emptyList(),
    @SerialName("waiting") val allWaiting: List<WaitingShareInvitation> = emptyList(),
    val completionError: String = "",
)

@Serializable
data class VaultSharing(
    val vault: VaultSummary,
    val isOwner: Boolean = false,
    @SerialName("members") val allMember: List<Member> = emptyList(),
    @SerialName("invitations") val allInvitation: List<SentShareInvitation> = emptyList(),
    val completionError: String = "",
)

/** What a push notification is about; the push carries nothing else. */
enum class PushKind(val value: String) {
    APPROVAL("approval"),
    SECURITY("security"),
    SHARING("sharing"),
    ;

    companion object {
        fun parse(text: String?): PushKind? = entries.firstOrNull { it.value == text }

        /** The kind in a Web Push message from a Hisar server, {"kind":"approval"}; null for anything else. */
        fun parseMessage(text: String): PushKind? = parseNotification(text)?.kind

        /**
         * The kind and optional invitation details in a Web Push message from a Hisar server; null for anything else.
         * One parser; older servers omit `invitation`, and unknown fields are ignored.
         */
        fun parseNotification(text: String): PushNotification? = try {
            val message = backendJson.decodeFromString(WebPushMessage.serializer(), text)
            parse(message.kind)?.let { PushNotification(it, message.invitation) }
        } catch (_: IllegalArgumentException) {
            null
        }
    }
}

/** The details of a sharing push, encrypted to this phone; absent on older servers. */
@Serializable
data class PushInvitation(
    val type: String = "",
    val inviterName: String = "",
    val inviterEmail: String = "",
    val organizationName: String = "",
    val role: String = "",
)

/** A decoded push: its kind and, for a sharing push, the invitation that names the inviter. */
data class PushNotification(val kind: PushKind, val invitation: PushInvitation?) {
    /**
     * The sentence that names the inviter and what they invited you to, or null to use the kind's own sentence (for no
     * invitation, an unknown type, an organization without a name, or a blank inviter). Identical to the iPhone's text.
     */
    fun detailText(): String? {
        val invitation = invitation ?: return null
        val inviter = inviterText(invitation.inviterName, invitation.inviterEmail)
        if (inviter.isEmpty()) return null
        return when (invitation.type) {
            "organization" -> invitation.organizationName.trim().takeIf { it.isNotEmpty() }
                ?.let { "$inviter invited you to join $it. Open Hisar to answer it." }
            "vault" -> "$inviter shared a vault with you. Open Hisar to answer it."
            else -> null
        }
    }

    private fun inviterText(name: String, email: String): String {
        val trimmedName = name.trim()
        val trimmedEmail = email.trim()
        return when {
            trimmedName.isEmpty() || trimmedName.equals(trimmedEmail, ignoreCase = true) -> trimmedEmail
            trimmedEmail.isEmpty() -> trimmedName
            else -> "$trimmedName ($trimmedEmail)"
        }
    }
}

@Serializable
private data class WebPushMessage(val kind: String = "", val invitation: PushInvitation? = null)

/** HisarKit's app.PushSettings: whether notifications can reach this phone, and the kinds turned on. */
@Serializable
data class PushSettings(
    val isAvailable: Boolean = false,
    val isServerSending: Boolean = false,
    val enabledKinds: List<String> = PushKind.entries.map { it.value },
    val errorMessage: String? = null,
) {
    fun isEnabled(kind: PushKind): Boolean = kind.value in enabledKinds

    companion object {
        val unavailable = PushSettings()
    }
}

/** A signed-in device of the account; the times are RFC 3339. */
@Serializable
data class Device(
    val id: String,
    val name: String = "",
    val platform: String = "",
    val createdAt: String = "",
    val lastSeenAt: String = "",
    val isCurrent: Boolean = false,
) {
    val lastSeenDate: Instant? get() = parseTimestamp(lastSeenAt)
    val createdDate: Instant? get() = parseTimestamp(createdAt)

    private val operatingSystem: String get() = platform.split("/").first()
    private val client: String get() = platform.split("/").getOrElse(1) { "" }

    val platformDescription: String
        get() {
            val system = when (operatingSystem) {
                "ios" -> "iOS"
                "android" -> "Android"
                "darwin", "macos" -> "macOS"
                "linux" -> "Linux"
                "windows" -> "Windows"
                else -> operatingSystem.ifEmpty { "Unknown" }
            }
            return when (client) {
                "mobile", "desktop" -> "$system app"
                "cli" -> "Command line on $system"
                "extension", "browser" -> "Browser extension on $system"
                else -> if (client.isEmpty()) system else "$system, $client"
            }
        }

    val icon: DeviceIcon
        get() = when {
            client == "cli" -> DeviceIcon.TERMINAL
            client == "extension" || client == "browser" -> DeviceIcon.EXTENSION
            operatingSystem == "ios" || operatingSystem == "android" -> DeviceIcon.PHONE
            operatingSystem == "darwin" || operatingSystem == "macos" -> DeviceIcon.LAPTOP
            else -> DeviceIcon.DESKTOP
        }
}

enum class DeviceIcon { PHONE, LAPTOP, DESKTOP, TERMINAL, EXTENSION }

@Serializable
data class EmergencyKitExport(val secretKey: String, val emergencyKitPath: String)

/** Who may rename, delete or leave a vault and what deleting it means; members leave out the user. */
@Serializable
data class VaultActions(
    val vault: VaultSummary,
    val canRename: Boolean = false,
    val canDelete: Boolean = false,
    val deleteBlockedReason: String = "",
    val canLeave: Boolean = false,
    val leaveBlockedReason: String = "",
    @SerialName("membersLosingAccess") val allMemberLosingAccess: List<Member> = emptyList(),
    @SerialName("targetVaults") val allTargetVault: List<VaultSummary> = emptyList(),
)

/** moveToVaultID moves every item first; isDeletingItems needs confirmedName, the vault's name. */
@Serializable
data class VaultDeletionRequest(
    @SerialName("moveToVaultId") val moveToVaultID: String = "",
    val isDeletingItems: Boolean = false,
    val confirmedName: String = "",
)

@Serializable
data class VaultDeletionResult(val movedCount: Int = 0, val targetVaultName: String = "")

/** A vault someone else deleted, or that the user lost access to. */
@Serializable
data class RemovedVaultNotice(val id: String, val name: String, val message: String)

@Serializable
data class ExportOptions(val isBiometricAvailable: Boolean, val minPassphraseLength: Int)

@Serializable
data class ExportResult(val path: String, val vaultCount: Int, val itemCount: Int, val warning: String = "")

enum class ExportFormat(val id: String, val label: String) {
    ENCRYPTED("encrypted", "Encrypted export"),
    PLAIN("plain", "Plain JSON"),
}

@Serializable
data class CredentialChangeResult(
    val status: VaultStatus,
    val secretKey: String = "",
    val signedOutDeviceCount: Int = 0,
    val keychainError: String = "",
    val localError: String = "",
    val emergencyKitPath: String = "",
    val pendingOperationId: String = "",
    val setupState: String = "",
    val problems: List<String> = emptyList(),
)

@Serializable
data class DeletionVault(val id: String, val name: String, val type: String, val role: String, val otherMemberCount: Int)

@Serializable
data class AccountDeletionPreview(
    val email: String = "",
    val planVersion: String = "",
    @SerialName("ownedVaults") val allOwnedVault: List<DeletionVault> = emptyList(),
    @SerialName("memberVaults") val allMemberVault: List<DeletionVault> = emptyList(),
    val shareLinkCount: Int = 0,
    val sentInvitationCount: Int = 0,
    val receivedInvitationCount: Int = 0,
    val deviceCount: Int = 1,
    @SerialName("organizations") val allOrganization: List<DeletionOrganization> = emptyList(),
) {
    val peopleLosingAccessCount: Int get() = allOwnedVault.sumOf { it.otherMemberCount }
}

/** isDeleted when the user is its last member; newAdminName is who becomes admin when the user is its last admin. */
@Serializable
data class DeletionOrganization(
    val id: String = "",
    val name: String = "",
    val isDeleted: Boolean = false,
    val newAdminName: String = "",
)

/** Organization roles; organization members are Members, with the same fields, security code included. */
object OrganizationRole {
    const val ADMIN = "admin"
    const val MEMBER = "member"
    val all = listOf(MEMBER, ADMIN)

    fun describe(role: String): String = if (role == ADMIN) "Admin" else "Member"

    /** "an admin" or "a member", as in "Ada invited you to Acme as a member". */
    fun describeWithArticle(role: String): String = if (role == ADMIN) "an admin" else "a member"
}

@Serializable
data class OrganizationSummary(
    val id: String,
    val name: String = "",
    val role: String = "",
    val memberCount: Int = 0,
) {
    val isAdmin: Boolean get() = role == OrganizationRole.ADMIN
}

/** An invitation to this user's email to join an organization; expiresAt is RFC 3339. */
@Serializable
data class OrganizationInvitation(
    val id: String,
    @SerialName("organizationId") val organizationID: String,
    val organizationName: String,
    val role: String,
    val invitedByName: String,
    val invitedByEmail: String,
    val expiresAt: String,
) {
    val inviterName: String get() = invitedByName.ifEmpty { invitedByEmail }
    val message: String get() = "$inviterName invited you to $organizationName as ${OrganizationRole.describeWithArticle(role)}"
}

@Serializable
data class Organizations(
    @SerialName("organizations") val allOrganization: List<OrganizationSummary> = emptyList(),
    @SerialName("invitations") val allInvitation: List<OrganizationInvitation> = emptyList(),
)

@Serializable
data class TeamMember(
    @SerialName("accountId") val accountID: String = "",
    val name: String = "",
    val email: String = "",
    val isCurrentUser: Boolean = false,
) {
    val id: String get() = accountID
    val displayName: String
        get() {
            val name = name.ifEmpty { email }
            return if (isCurrentUser) "$name (you)" else name
        }
}

/**
 * A vault the team was given. Vault names are encrypted, so name is empty when canOpen is false: this device has no key for
 * it (yet).
 */
@Serializable
data class TeamVault(
    @SerialName("vaultId") val vaultID: String = "",
    val name: String = "",
    val role: String = "",
    val canOpen: Boolean = false,
) {
    val id: String get() = vaultID
    val displayName: String get() = if (canOpen) name else UNOPENABLE_NAME

    companion object {
        const val UNOPENABLE_NAME = "A vault you can't open"
    }
}

@Serializable
data class Team(
    val id: String,
    val name: String = "",
    @SerialName("members") val allMember: List<TeamMember> = emptyList(),
    @SerialName("vaults") val allVault: List<TeamVault> = emptyList(),
) {
    val hasCurrentUser: Boolean get() = allMember.any { it.isCurrentUser }
}

/** A pending invitation to the organization; only admins see them. Times are RFC 3339. */
@Serializable
data class SentOrganizationInvitation(
    val id: String,
    val email: String,
    val role: String,
    val createdAt: String,
    val expiresAt: String,
)

@Serializable
data class Organization(
    val id: String,
    val name: String = "",
    val role: String = "",
    val isAdmin: Boolean = false,
    @SerialName("members") val allMember: List<Member> = emptyList(),
    @SerialName("teams") val allTeam: List<Team> = emptyList(),
    /** Empty unless isAdmin. */
    @SerialName("invitations") val allInvitation: List<SentOrganizationInvitation> = emptyList(),
)

/** An organization vault this user's teams give them that no owner's Hisar has delivered yet. */
@Serializable
data class WaitingVaultKey(
    @SerialName("vaultId") val vaultID: String,
    @SerialName("organizationId") val organizationID: String,
    val organizationName: String,
    val role: String,
) {
    val id: String get() = vaultID
}

/**
 * deliveryError and rotationError say why this app could not give someone a vault key or rotate a vault it owns, e.g. a
 * changed signing key.
 */
@Serializable
data class WaitingVaultKeys(
    @SerialName("waiting") val allWaiting: List<WaitingVaultKey> = emptyList(),
    val deliveryError: String = "",
    val rotationError: String = "",
)

/** An item an import leaves out or failed to save, with why. */
@Serializable
data class ImportSkippedEntry(val title: String = "", val reason: String = "") {
    companion object {
        fun heading(count: Int): String = "Not imported · $count"
        const val EXPLANATION = "Some entries below are fields or credentials from items that will still be imported."
    }
}

/**
 * An item the chosen vault already has. passkeyCount is how many of its passkeys the existing login lacks; skipping
 * duplicates adds them to it.
 */
@Serializable
data class ImportDuplicate(
    val title: String = "",
    val username: String = "",
    val url: String = "",
    val detail: String = "",
    val passkeyCount: Int = 0,
)

/**
 * What an import would bring into vaultID. For a Credential Exchange import fileName names the app the items came from
 * and filePath is empty.
 */
@Serializable
data class ImportSourceGroup(
    val id: String = "",
    val name: String = "",
    val isPrivate: Boolean = true,
    val itemCount: Int = 0,
    @SerialName("vaultId") val vaultID: String = "",
    val vaultName: String = "",
    val members: List<Member> = emptyList(),
)

@Serializable
data class ImportPreview(
    val fileName: String = "",
    val filePath: String = "",
    val format: String = FORMAT_CREDENTIAL_EXCHANGE,
    val formatName: String = "Credential Exchange",
    @SerialName("vaultId") val vaultID: String = "",
    val vaultName: String = "",
    val itemCount: Int = 0,
    val loginCount: Int = 0,
    val secureNoteCount: Int = 0,
    val sshKeyCount: Int = 0,
    val creditCardCount: Int = 0,
    val identityCount: Int = 0,
    val apiCredentialCount: Int = 0,
    val addressCount: Int = 0,
    val softwareLicenseCount: Int = 0,
    val serverCount: Int = 0,
    val databaseCount: Int = 0,
    val passkeyCount: Int = 0,
    val allSkipped: List<ImportSkippedEntry> = emptyList(),
    val allDuplicate: List<ImportDuplicate> = emptyList(),
    val groups: List<ImportSourceGroup> = emptyList(),
    /** The file is an encrypted Hisar export; openImportFile opens it. */
    val isPassphraseRequired: Boolean = false,
    /** A warning for the preview when non-empty, e.g. a cached or partial Hisar export. */
    val notice: String = "",
    /** The source is kept rather than offered for deletion: an encrypted export or a cached or partial copy. */
    val isSourceKept: Boolean = false,
    /** The account the Hisar export came from, or empty. */
    val exportedFrom: String = "",
    /** When the Hisar export was made, RFC 3339 UTC, or empty. */
    val exportedAt: String = "",
) {
    val isHisarExport: Boolean get() = format == "hisar-export" || format == "hisar-encrypted-export"

    val groupDestinationExplanation: String
        get() = if (isHisarExport) {
            "This export has ${groups.size} vaults. Each goes to the vault it came from, or your Personal vault, unless you choose another."
        } else {
            "The other app kept these items in ${groups.size} groups. Each goes to your Personal vault unless you choose another vault for it."
        }

    /** The preview's non-zero counts, as the import sheet lists them. */
    val allCountRow: List<ImportCountRow>
        get() = listOf(
            ImportCountRow("Logins", loginCount), ImportCountRow("Passkeys", passkeyCount),
            ImportCountRow("Secure notes", secureNoteCount), ImportCountRow("Cards", creditCardCount),
            ImportCountRow("Identities", identityCount), ImportCountRow("SSH keys", sshKeyCount),
            ImportCountRow("API credentials", apiCredentialCount), ImportCountRow("Addresses", addressCount),
            ImportCountRow("Software licenses", softwareLicenseCount), ImportCountRow("Servers", serverCount),
            ImportCountRow("Databases", databaseCount),
        ).filter { it.count > 0 }

    /** How many passkeys skipping the duplicates still adds to the logins the vault already has. */
    val duplicatePasskeyCount: Int get() = allDuplicate.sumOf { it.passkeyCount }

    companion object {
        const val FORMAT_CREDENTIAL_EXCHANGE = "credential-exchange"
    }
}

data class ImportCountRow(val label: String, val count: Int)

/** A running import: continueImport saves the next items until isDone. */
@Serializable
data class ImportProgress(
    val fileName: String = "",
    val filePath: String = "",
    @SerialName("vaultId") val vaultID: String = "",
    val vaultName: String = "",
    val savedCount: Int = 0,
    val totalCount: Int = 0,
    val isDone: Boolean = false,
    val allFailed: List<ImportSkippedEntry> = emptyList(),
    val allSkipped: List<ImportSkippedEntry> = emptyList(),
    val allDuplicateOutcomes: List<ImportSkippedEntry> = emptyList(),
    val leftBehindCount: Int = 0,
    /** The source is kept rather than offered for deletion, see [ImportPreview.isSourceKept]. */
    val isSourceKept: Boolean = false,
)
