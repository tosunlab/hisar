package com.tryhisar.hisar.ui

import android.graphics.Matrix
import android.os.Bundle
import android.os.LocaleList
import android.view.View
import android.view.ViewStructure
import android.view.autofill.AutofillId
import android.view.autofill.AutofillValue

/**
 * What a view tells an autofill service about itself and its virtual children, as the framework records it before it
 * sends the structure to the service: whether a node can be filled, its value, and whether that value is sensitive.
 */
class RecordedViewStructure : ViewStructure() {
    var autofillType = View.AUTOFILL_TYPE_NONE
        private set
    var autofillValue: AutofillValue? = null
        private set
    var isSensitive = false
        private set
    private var allChild = arrayOfNulls<RecordedViewStructure>(0)
    private val extras = Bundle()

    /** This node and every node below it. */
    fun listAllNode(): List<RecordedViewStructure> = listOf(this) + allChild.filterNotNull().flatMap { it.listAllNode() }

    /** The node whose autofill value is [text], ignoring spaces, such as those a card number field groups digits with. */
    fun findNodeWithValue(text: String): RecordedViewStructure? = listAllNode().singleOrNull { node ->
        node.autofillValue?.takeIf(AutofillValue::isText)?.textValue?.filterNot(Char::isWhitespace)?.toString() == text.filterNot(Char::isWhitespace)
    }

    override fun setAutofillType(type: Int) {
        autofillType = type
    }

    override fun setAutofillValue(value: AutofillValue?) {
        autofillValue = value
    }

    override fun setDataIsSensitive(sensitive: Boolean) {
        isSensitive = sensitive
    }

    override fun setChildCount(num: Int) {
        allChild = allChild.copyOf(num)
    }

    override fun addChildCount(num: Int): Int {
        val oldCount = allChild.size
        allChild = allChild.copyOf(oldCount + num)
        return oldCount
    }

    override fun getChildCount() = allChild.size

    override fun newChild(index: Int): ViewStructure = RecordedViewStructure().also { allChild[index] = it }

    override fun asyncNewChild(index: Int): ViewStructure = newChild(index)

    override fun asyncCommit() = Unit

    override fun getExtras() = extras

    override fun hasExtras() = !extras.isEmpty

    override fun getAutofillId(): AutofillId? = null

    override fun setAutofillId(id: AutofillId) = Unit

    override fun setAutofillId(parentId: AutofillId, virtualId: Int) = Unit

    override fun setAutofillHints(hint: Array<out String>?) = Unit

    override fun setAutofillOptions(options: Array<out CharSequence>?) = Unit

    override fun setInputType(inputType: Int) = Unit

    override fun newHtmlInfoBuilder(tagName: String): HtmlInfo.Builder = throw UnsupportedOperationException("Compose has no HTML")

    override fun setHtmlInfo(htmlInfo: HtmlInfo) = Unit

    override fun setWebDomain(domain: String?) = Unit

    override fun setLocaleList(localeList: LocaleList?) = Unit

    override fun setId(id: Int, packageName: String?, typeName: String?, entryName: String?) = Unit

    override fun setDimens(left: Int, top: Int, scrollX: Int, scrollY: Int, width: Int, height: Int) = Unit

    override fun setTransformation(matrix: Matrix?) = Unit

    override fun setElevation(elevation: Float) = Unit

    override fun setAlpha(alpha: Float) = Unit

    override fun setVisibility(visibility: Int) = Unit

    override fun setEnabled(state: Boolean) = Unit

    override fun setClickable(state: Boolean) = Unit

    override fun setLongClickable(state: Boolean) = Unit

    override fun setContextClickable(state: Boolean) = Unit

    override fun setFocusable(state: Boolean) = Unit

    override fun setFocused(state: Boolean) = Unit

    override fun setAccessibilityFocused(state: Boolean) = Unit

    override fun setCheckable(state: Boolean) = Unit

    override fun setChecked(state: Boolean) = Unit

    override fun setSelected(state: Boolean) = Unit

    override fun setActivated(state: Boolean) = Unit

    override fun setOpaque(opaque: Boolean) = Unit

    override fun setClassName(className: String?) = Unit

    override fun setContentDescription(contentDescription: CharSequence?) = Unit

    override fun setText(text: CharSequence?) = Unit

    override fun setText(text: CharSequence?, selectionStart: Int, selectionEnd: Int) = Unit

    override fun setTextStyle(size: Float, fgColor: Int, bgColor: Int, style: Int) = Unit

    override fun setTextLines(charOffsets: IntArray?, baselines: IntArray?) = Unit

    override fun setHint(hint: CharSequence?) = Unit

    override fun getText(): CharSequence? = null

    override fun getTextSelectionStart() = -1

    override fun getTextSelectionEnd() = -1

    override fun getHint(): CharSequence? = null
}
