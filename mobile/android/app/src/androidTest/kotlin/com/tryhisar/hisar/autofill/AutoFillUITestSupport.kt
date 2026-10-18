package com.tryhisar.hisar.autofill

import androidx.test.core.app.ApplicationProvider
import com.tryhisar.hisar.HisarApplication
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultBackend
import org.junit.Assert.assertTrue

/** The app's own HisarKit, which the AutoFill screens and service share with the app. */
val testBackend: VaultBackend get() = ApplicationProvider.getApplicationContext<HisarApplication>().backend

/** Signs up with the harness invite and saves the logins through HisarKit; the vault screens belong to other tests. */
fun HisarUITestCase.signUpWithLogins(accountPrefix: String, vararg allLogin: ItemDetail) {
    val signedUp = testBackend.signUp(readArgument("HISAR_UI_TEST_${accountPrefix}INVITE_LINK"), readArgument("HISAR_UI_TEST_${accountPrefix}EMAIL"), "Android AutoFill", HisarUITestCase.MASTER_PASSWORD)
    assertTrue("signed up: ${signedUp.status}", signedUp.status.isUnlocked)
    val personal = testBackend.listAllVault().single { it.name == "Personal" }
    for (login in allLogin) testBackend.saveItem(personal.id, login)
}

fun login(title: String, username: String, password: String, website: String, oneTimePassword: String = "") = ItemDetail(
    category = ItemDetail.CATEGORY_LOGIN, title = title, username = username, password = password, allURL = listOf(website), oneTimePassword = oneTimePassword,
)
