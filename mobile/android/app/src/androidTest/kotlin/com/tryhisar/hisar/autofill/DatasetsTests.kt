package com.tryhisar.hisar.autofill

import android.graphics.Rect
import android.view.ContextThemeWrapper
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.tryhisar.hisar.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DatasetsTests {
    @Test
    fun testSuggestionsShowHisarMarkAndReadableTitleInBothThemes() {
        val targetContext = InstrumentationRegistry.getInstrumentation().targetContext
        for (theme in listOf(android.R.style.Theme_Material_Light, android.R.style.Theme_Material)) {
            val context = ContextThemeWrapper(targetContext, theme)
            val root = Datasets(context, null, 0).buildPresentation(Suggestion("Unlock Hisar", ""))
                .apply(context, null) as ViewGroup
            val icon = root.findViewById<ImageView>(R.id.autofill_suggestion_icon)
            assertNotNull(icon.drawable)
            root.measure(
                android.view.View.MeasureSpec.makeMeasureSpec(1080, android.view.View.MeasureSpec.EXACTLY),
                android.view.View.MeasureSpec.makeMeasureSpec(0, android.view.View.MeasureSpec.UNSPECIFIED),
            )
            root.layout(0, 0, root.measuredWidth, root.measuredHeight)
            val title = root.findViewById<TextView>(R.id.autofill_suggestion_title)
            val iconBounds = Rect(0, 0, icon.width, icon.height).also { root.offsetDescendantRectToMyCoords(icon, it) }
            val titleBounds = Rect(0, 0, title.width, title.height).also { root.offsetDescendantRectToMyCoords(title, it) }
            assertTrue("the Hisar mark precedes the suggestion title", iconBounds.right <= titleBounds.left)
            val attributes = context.obtainStyledAttributes(intArrayOf(android.R.attr.textColorPrimary))
            try {
                assertEquals(attributes.getColor(0, 0), title.currentTextColor)
            } finally {
                attributes.recycle()
            }
            assertEquals("Unlock Hisar", title.text.toString())
        }
    }
}
