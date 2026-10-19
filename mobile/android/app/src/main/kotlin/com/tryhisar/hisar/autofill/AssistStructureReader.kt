package com.tryhisar.hisar.autofill

import android.app.assist.AssistStructure
import android.view.View
import android.view.autofill.AutofillId

/** The views of a fill request as the JVM parser reads them; [focusedID] is the view the request is for. */
fun AssistStructure.readFormNodes(focusedID: AutofillId): List<FormNode<AutofillId>> =
    (0 until windowNodeCount).map { readNode(getWindowNodeAt(it).rootViewNode, focusedID) }

private fun readNode(node: AssistStructure.ViewNode, focusedID: AutofillId): FormNode<AutofillId> = FormNode(
    autofillID = node.autofillId,
    isTextField = node.autofillType == View.AUTOFILL_TYPE_TEXT,
    isFocused = node.autofillId == focusedID,
    autofillHints = node.autofillHints?.toList().orEmpty(),
    htmlTag = node.htmlInfo?.tag,
    htmlAttributes = node.htmlInfo?.attributes?.associate { it.first to it.second }.orEmpty(),
    inputType = node.inputType,
    idEntry = node.idEntry,
    hint = node.hint,
    webDomain = node.webDomain,
    webScheme = node.webScheme,
    children = (0 until node.childCount).map { readNode(node.getChildAt(it), focusedID) },
)

/** The text the user typed into each field, for the save prompt. */
fun AssistStructure.readText(id: AutofillId): String? {
    fun find(node: AssistStructure.ViewNode): String? {
        if (node.autofillId == id) return node.autofillValue?.takeIf { it.isText }?.textValue?.toString()
        return (0 until node.childCount).firstNotNullOfOrNull { find(node.getChildAt(it)) }
    }
    return (0 until windowNodeCount).firstNotNullOfOrNull { find(getWindowNodeAt(it).rootViewNode) }
}
