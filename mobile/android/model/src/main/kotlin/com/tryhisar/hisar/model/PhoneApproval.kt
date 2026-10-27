package com.tryhisar.hisar.model

import java.time.Duration
import java.time.Instant
import kotlin.io.encoding.Base64
import kotlin.math.ceil
import kotlinx.io.bytestring.ByteString
import kotlinx.io.bytestring.decodeToByteString
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.nullable
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

/**
 * A request the user's Mac is waiting on: approve or deny one SSH or command-line use, one share link an app such as
 * Raycast creates, or that app using Hisar until it locks on the Mac.
 */
@Serializable
data class PhoneApproval(
    @SerialName("requestId") val requestID: String,
    val requesterDeviceName: String = "",
    val createdAt: String = "",
    val expiresAt: String = "",
    val kind: String = "",
    val appName: String = "",
    val purpose: String = "",
    val host: String = "",
    val commandLine: String = "",
    val repositoryPath: String = "",
    /** The folder of the coding agent's harness, such as omp's project; empty from an older Mac. */
    val projectPath: String = "",
    val keyName: String = "",
    val isForwarded: Boolean = false,
    val forwardedFromHost: String = "",
    val terminalName: String = "",
    val secrets: List<String> = emptyList(),
    @Serializable(with = AppIconSerializer::class) val appIcon: ByteString? = null,
    val appWarning: String = "",
    /** Set when approving can also be remembered on the Mac; the sheet and the Vaults card show it as a switch. */
    val rememberLabel: String = "",
    val isRememberedByDefault: Boolean = false,
) {
    data class DetailRow(val label: String, val value: String, val isMonospaced: Boolean, val isCompact: Boolean = false) {
        val id: String get() = label
    }

    val id: String get() = requestID

    val isRememberable: Boolean get() = rememberLabel.isNotEmpty()

    /** The switch as the user left it, or its default; a request without the switch is never remembered. */
    fun resolveRememberChoice(chosenValue: Boolean?): Boolean = isRememberable && (chosenValue ?: isRememberedByDefault)

    /** The Vaults card's footer: the card's switch says what else approving allows, so the footer says only what it never does. */
    val cardApprovalNote: String
        get() = if (isRememberable) {
            "It never unlocks Hisar on your Mac."
        } else {
            "Approving answers this one request. It never unlocks Hisar on your Mac."
        }

    val expiryDate: Instant? get() = parseTimestamp(expiresAt)

    val macName: String get() = requesterDeviceName.ifEmpty { "your Mac" }

    val title: String get() = "Approve on $macName?"

    val summary: String
        get() {
            val requester = if (isForwarded) forwardedFromHost.ifEmpty { "Another computer" } else appName
            return "$requester wants to $purpose"
        }

    val bannerTitle: String
        get() = when (kind) {
            KIND_COMMAND_LINE -> "$macName is asking to use Hisar from the command line"
            KIND_LAUNCHER -> "$macName is asking to let $appName use Hisar"
            else -> "$macName is asking to use ${if (keyName.isEmpty()) "your SSH key" else "“$keyName”"}"
        }

    /**
     * Set for an approval that outlasts the request: a launcher keeps its approval until Hisar locks on the Mac. A launcher
     * request that names secrets is a share link, which is approved once like any other request.
     */
    val lifetime: String get() = if (kind == KIND_LAUNCHER && secrets.isEmpty()) "Until Hisar locks on $macName" else ""

    val approvalNote: String
        get() = if (lifetime.isNotEmpty()) {
            "Approving lets $appName use Hisar until it locks on your Mac; it does not unlock Hisar there."
        } else {
            "Approving lets this one request continue; it does not unlock Hisar on your Mac."
        }

    val detailLine: String
        get() = when {
            keyName.isNotEmpty() -> "with “$keyName”"
            secrets.isNotEmpty() -> secrets.joinToString(", ")
            lifetime.isNotEmpty() -> lifetime
            else -> commandLine
        }

    fun formatTimeLeft(date: Instant): String {
        val secondCount = maxOf(0, ceil(Duration.between(date, expiryDate ?: date).toMillis() / 1000.0).toInt())
        if (secondCount < 60) return if (secondCount == 1) "Expires in 1 second" else "Expires in $secondCount seconds"
        return "Expires in %d:%02d".format(secondCount / 60, secondCount % 60)
    }

    val icon: PhoneApprovalIcon
        get() = when (kind) {
            KIND_COMMAND_LINE -> PhoneApprovalIcon.TERMINAL
            KIND_LAUNCHER -> PhoneApprovalIcon.LAUNCHER
            else -> PhoneApprovalIcon.KEY
        }

    val allDetailRow: List<DetailRow>
        get() {
            val appRow = DetailRow("App", appName, false, isCompact = true)
            val secretRow = DetailRow("Secrets", secrets.joinToString("\n"), false)
            // The sheet shows appWarning on its own above these rows.
            val allCandidate = if (kind == KIND_LAUNCHER) {
                listOf(appRow, secretRow, DetailRow("Link", commandLine, false), DetailRow("Lasts", lifetime, false, isCompact = true))
            } else {
                listOf(
                    appRow,
                    DetailRow("Project", projectPath, true),
                    DetailRow(if (kind == KIND_COMMAND_LINE && secrets.isEmpty()) "Program" else "Command", commandLine, true),
                    DetailRow("Repository", if (isRepositoryTheProject) "" else repositoryPath, true),
                    DetailRow("Server", host, true),
                    DetailRow("Key", keyName, false, isCompact = true),
                    secretRow,
                    DetailRow("Forwarded from", if (isForwarded) forwardedFromHost else "", true),
                    DetailRow("Terminal", if (terminalName.isEmpty()) "" else "/dev/$terminalName", true),
                )
            }
            return allCandidate.filter { it.value.isNotEmpty() }
        }

    // A coding agent usually runs in its project's root, where the repository row would only repeat the project.
    private val isRepositoryTheProject: Boolean
        get() = projectPath.isNotEmpty() && repositoryPath.removeSuffix("/") == projectPath.removeSuffix("/")

    fun isExpired(date: Instant): Boolean {
        val expiryDate = expiryDate ?: return true
        return !expiryDate.isAfter(date)
    }

    companion object {
        const val KIND_SSH = "ssh"
        const val KIND_COMMAND_LINE = "cli"
        const val KIND_LAUNCHER = "launcher"
        const val EVENT_REQUESTED = "approval-requested"
        const val EVENT_ANSWERED = "approval-answered"
        const val EVENT_RESOLVED = "approval-resolved"
    }
}

/** The symbol of a request's kind: an SSH key, the command line, or an app such as Raycast. */
enum class PhoneApprovalIcon { KEY, TERMINAL, LAUNCHER }

// An icon that is not base64 is dropped, as the request is still worth answering without it.
@OptIn(ExperimentalSerializationApi::class)
private object AppIconSerializer : KSerializer<ByteString?> {
    override val descriptor = PrimitiveSerialDescriptor("com.tryhisar.hisar.model.AppIcon", PrimitiveKind.STRING).nullable

    override fun serialize(encoder: Encoder, value: ByteString?) {
        if (value == null) encoder.encodeNull() else Base64ByteStringSerializer.serialize(encoder, value)
    }

    override fun deserialize(decoder: Decoder): ByteString? {
        if (!decoder.decodeNotNullMark()) return decoder.decodeNull()
        return try {
            Base64.decodeToByteString(decoder.decodeString())
        } catch (_: IllegalArgumentException) {
            null
        }
    }
}
