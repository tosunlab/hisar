package com.tryhisar.hisar.model

// Swift's CharacterSet.whitespaces: tab and the space separators, without line breaks.
internal fun Char.isSpaceOrTab(): Boolean = this == '\t' || Character.getType(this) == Character.SPACE_SEPARATOR.toInt()

internal fun String.trimmingWhitespaces(): String = trim { it.isSpaceOrTab() }

internal fun String.trimmingWhitespacesAndNewlines(): String = trim { it.isWhitespace() }

// Swift's Character.isNumber: decimal digits and other numeric characters such as Roman numerals.
internal fun Char.isNumber(): Boolean = when (Character.getType(this).toByte()) {
    Character.DECIMAL_DIGIT_NUMBER, Character.LETTER_NUMBER, Character.OTHER_NUMBER -> true
    else -> false
}

internal fun Char.isAsciiDigit(): Boolean = this in '0'..'9'
