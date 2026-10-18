package com.tryhisar.hisar.ui.vaults

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AccountBalance
import androidx.compose.material.icons.outlined.AccountBalanceWallet
import androidx.compose.material.icons.outlined.AlternateEmail
import androidx.compose.material.icons.outlined.Badge
import androidx.compose.material.icons.outlined.CardGiftcard
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.Cloud
import androidx.compose.material.icons.outlined.Code
import androidx.compose.material.icons.outlined.CreditCard
import androidx.compose.material.icons.outlined.CurrencyBitcoin
import androidx.compose.material.icons.outlined.DataObject
import androidx.compose.material.icons.outlined.DirectionsCar
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.FavoriteBorder
import androidx.compose.material.icons.outlined.Flight
import androidx.compose.material.icons.outlined.HealthAndSafety
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.MusicNote
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.School
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.ShoppingCart
import androidx.compose.material.icons.outlined.SmartDisplay
import androidx.compose.material.icons.outlined.SportsEsports
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material.icons.automirrored.outlined.StickyNote2
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.Verified
import androidx.compose.material.icons.outlined.Wifi
import androidx.compose.material.icons.outlined.Work
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.WebsiteIconModel
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.io.bytestring.ByteString

/** The symbols and colors of item tiles, in place of the iPhone app's SF Symbols and system colors. */
object ItemIconStyle {
    private val iconByPreset: Map<String, ImageVector> = mapOf(
        "bank" to Icons.Outlined.AccountBalance,
        "card" to Icons.Outlined.CreditCard,
        "wallet" to Icons.Outlined.AccountBalanceWallet,
        "crypto" to Icons.Outlined.CurrencyBitcoin,
        "cart" to Icons.Outlined.ShoppingCart,
        "gift" to Icons.Outlined.CardGiftcard,
        "mail" to Icons.Outlined.Email,
        "chat" to Icons.Outlined.ChatBubbleOutline,
        "social" to Icons.Outlined.AlternateEmail,
        "work" to Icons.Outlined.Work,
        "home" to Icons.Outlined.Home,
        "school" to Icons.Outlined.School,
        "cloud" to Icons.Outlined.Cloud,
        "server" to Icons.Outlined.Dns,
        "database" to Icons.Outlined.Storage,
        "code" to Icons.Outlined.Code,
        "terminal" to Icons.Outlined.Terminal,
        "wifi" to Icons.Outlined.Wifi,
        "globe" to Icons.Outlined.Public,
        "game" to Icons.Outlined.SportsEsports,
        "music" to Icons.Outlined.MusicNote,
        "video" to Icons.Outlined.SmartDisplay,
        "health" to Icons.Outlined.HealthAndSafety,
        "travel" to Icons.Outlined.Flight,
        "car" to Icons.Outlined.DirectionsCar,
        "key" to Icons.Outlined.Key,
        "lock" to Icons.Outlined.Lock,
        "shield" to Icons.Outlined.Shield,
        "star" to Icons.Outlined.StarOutline,
        "heart" to Icons.Outlined.FavoriteBorder,
    )

    /** iOS's system colors, light and dark, for the icon color IDs of core/item. */
    private val colorByID: Map<String, Pair<Long, Long>> = mapOf(
        "red" to (0xFFFF3B30 to 0xFFFF453A),
        "orange" to (0xFFFF9500 to 0xFFFF9F0A),
        "yellow" to (0xFFFFCC00 to 0xFFFFD60A),
        "green" to (0xFF34C759 to 0xFF30D158),
        "teal" to (0xFF30B0C7 to 0xFF40C8E0),
        "blue" to (0xFF007AFF to 0xFF0A84FF),
        "indigo" to (0xFF5856D6 to 0xFF5E5CE6),
        "purple" to (0xFFAF52DE to 0xFFBF5AF2),
        "pink" to (0xFFFF2D55 to 0xFFFF375F),
        "gray" to (0xFF8E8E93 to 0xFF8E8E93),
    )

    fun getPresetIcon(preset: String): ImageVector? = iconByPreset[preset]

    /** The color of a preset; null for the neutral tile. */
    fun getColor(id: String, isDark: Boolean = false): Color? = colorByID[id]?.let { Color(if (isDark) it.second else it.first) }

    /** The symbol of a category that shows one; a login shows its initial. */
    fun getCategoryIcon(item: ItemDetail): ImageVector? = when {
        item.isSSHKey -> Icons.Outlined.Key
        item.isSecureNote -> Icons.AutoMirrored.Outlined.StickyNote2
        item.isCreditCard -> Icons.Outlined.CreditCard
        item.isIdentity -> Icons.Outlined.Badge
        item.isAPICredential -> Icons.Outlined.DataObject
        item.isAddress -> Icons.Outlined.Place
        item.isSoftwareLicense -> Icons.Outlined.Verified
        item.isServer -> Icons.Outlined.Dns
        item.isDatabase -> Icons.Outlined.Storage
        else -> null
    }
}

/**
 * The tile of an item: the image or preset the user chose, else a login's website icon (which [AppItemIcon] loads and
 * passes in), else the category's symbol or the title's initial.
 */
@Composable
fun ItemIcon(item: ItemDetail, size: Dp = 32.dp, websiteIcon: ImageBitmap? = null) {
    val colors = HisarTheme.colors
    val shape = RoundedCornerShape(size * 0.25f)
    val isLarge = size > 40.dp
    val chosenImage = rememberImageBitmap(item.icon?.image)
    val image = chosenImage ?: websiteIcon.takeIf { item.icon == null }
    val preset = item.icon?.drawnPreset
    val presetColor = item.icon?.color?.let { ItemIconStyle.getColor(it, colors.isDark) }
    val background = when {
        image != null -> Color.White
        preset != null && presetColor != null -> presetColor.copy(alpha = 0.18f)
        else -> colors.fill
    }
    Box(Modifier.size(size).clip(shape).background(background), contentAlignment = Alignment.Center) {
        val symbolSize = if (isLarge) 28.dp else 17.dp
        val presetIcon = preset?.let(ItemIconStyle::getPresetIcon)
        when {
            image != null -> Image(image, contentDescription = null, contentScale = ContentScale.Fit, modifier = Modifier.fillMaxSize())
            presetIcon != null -> Icon(presetIcon, contentDescription = null, tint = presetColor ?: colors.secondaryLabel, modifier = Modifier.size(symbolSize))
            else -> {
                val categoryIcon = ItemIconStyle.getCategoryIcon(item)
                if (categoryIcon != null) {
                    Icon(categoryIcon, contentDescription = null, tint = colors.secondaryLabel, modifier = Modifier.size(symbolSize))
                } else {
                    Text(
                        item.title.firstOrNull()?.uppercase() ?: "?",
                        fontSize = if (isLarge) 28.sp else 15.sp,
                        fontWeight = FontWeight.SemiBold,
                        color = colors.secondaryLabel,
                    )
                }
            }
        }
    }
}

/** [ItemIcon] with the website icon a login shows, which the app model loads once per domain. */
@Composable
fun AppItemIcon(model: AppModel, item: ItemDetail, size: Dp = 32.dp) {
    val websiteURL = item.websiteIconURL
    val iconByDomain by model.websiteIcons.iconByDomain.collectAsStateWithLifecycle()
    LaunchedEffect(websiteURL) { websiteURL?.let(model.websiteIcons::loadWebsiteIcon) }
    val websiteIcon = websiteURL?.let(WebsiteIconModel::getWebsiteIconDomain)?.let(iconByDomain::get)
    ItemIcon(item, size, rememberImageBitmap(websiteIcon))
}

@Composable
private fun rememberImageBitmap(data: ByteString?): ImageBitmap? = remember(data) {
    data?.toByteArray()?.let { bytes -> BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap() }
}
