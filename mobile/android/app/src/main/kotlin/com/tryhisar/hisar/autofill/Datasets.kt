@file:Suppress("DEPRECATION")

package com.tryhisar.hisar.autofill

import android.app.PendingIntent
import android.app.slice.Slice
import android.content.Context
import android.content.Intent
import android.os.Build
import android.service.autofill.Dataset
import android.service.autofill.InlinePresentation
import android.view.View
import android.view.autofill.AutofillId
import android.view.autofill.AutofillValue
import android.view.inputmethod.InlineSuggestionsRequest
import android.widget.RemoteViews
import android.widget.inline.InlinePresentationSpec
import androidx.autofill.inline.UiVersions
import androidx.autofill.inline.v1.InlineSuggestionUi
import com.tryhisar.hisar.R

/** One suggestion as the system shows it: in the dropdown, and above the keyboard on API 30 and later. */
class Suggestion(val title: String, val subtitle: String)

/**
 * Builds the datasets of a fill response. A suggestion fills nothing by itself: picking it opens [AutofillActivity],
 * which hands the system the values of that one login, so a secret leaves Hisar only for the login the user picked.
 */
class Datasets(private val context: Context, private val inlineRequest: InlineSuggestionsRequest?) {
    private var requestCode = 0

    /** How many suggestions the keyboard takes; the dropdown takes any number. */
    val inlineLimit: Int? get() = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) inlineRequest?.maxSuggestionCount else null

    fun buildSuggestion(request: AutofillRequest, suggestion: Suggestion, index: Int): Dataset {
        val intent = request.writeTo(Intent(context, AutofillActivity::class.java))
        // The system adds the request's structure to the intent, so it stays mutable; it names Hisar's own activity.
        val pendingIntent = PendingIntent.getActivity(context, requestCode++, intent, PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_CANCEL_CURRENT)
        val presentation = buildPresentation(suggestion)
        val inline = buildInlinePresentation(suggestion, index, pendingIntent)
        val builder = Dataset.Builder(presentation)
        for (id in listOfNotNull(request.username, request.password, request.oneTimeCode)) {
            setValue(builder, id, null, presentation, inline)
        }
        return builder.setAuthentication(pendingIntent.intentSender).build()
    }

    fun buildPresentation(suggestion: Suggestion): RemoteViews = RemoteViews(context.packageName, R.layout.autofill_suggestion).apply {
        setTextViewText(R.id.autofill_suggestion_title, suggestion.title)
        setTextViewText(R.id.autofill_suggestion_subtitle, suggestion.subtitle)
        setViewVisibility(R.id.autofill_suggestion_subtitle, if (suggestion.subtitle.isEmpty()) View.GONE else View.VISIBLE)
    }

    private fun buildInlinePresentation(suggestion: Suggestion, index: Int, pendingIntent: PendingIntent): InlinePresentation? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return null
        val allSpec = inlineRequest?.inlinePresentationSpecs.orEmpty()
        if (allSpec.isEmpty() || index >= inlineRequest!!.maxSuggestionCount) return null
        val spec: InlinePresentationSpec = allSpec[minOf(index, allSpec.lastIndex)]
        if (!UiVersions.getVersions(spec.style).contains(UiVersions.INLINE_UI_VERSION_1)) return null
        val slice: Slice = InlineSuggestionUi.newContentBuilder(pendingIntent)
            .setTitle(suggestion.title)
            .apply { if (suggestion.subtitle.isNotEmpty()) setSubtitle(suggestion.subtitle) }
            .setContentDescription(listOf(suggestion.title, suggestion.subtitle).filter { it.isNotEmpty() }.joinToString(", "))
            .build().slice
        return InlinePresentation(slice, spec, false)
    }

    companion object {
        /** The values of the login the user picked, as [AutofillActivity] returns them to the system. */
        fun buildFilled(context: Context, allValue: List<Pair<AutofillId, String>>, title: String): Dataset {
            val presentation = Datasets(context, null).buildPresentation(Suggestion(title, ""))
            val builder = Dataset.Builder(presentation)
            for ((id, value) in allValue) setValue(builder, id, AutofillValue.forText(value), presentation, null)
            return builder.build()
        }

        // Field and Presentations replace these setters on API 33; the older ones keep one path for API 29 to 37.
        private fun setValue(builder: Dataset.Builder, id: AutofillId, value: AutofillValue?, presentation: RemoteViews, inline: InlinePresentation?) {
            if (inline != null && Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                builder.setValue(id, value, presentation, inline)
            } else {
                builder.setValue(id, value, presentation)
            }
        }
    }
}
