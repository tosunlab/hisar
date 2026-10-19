package com.tryhisar.hisar.model

/** The detail screen's line under the title: the category, or the summary a card, identity, credential or address shows. */
val ItemDetail.categoryDescription: String
    get() = when {
        isSSHKey -> "SSH key"
        isSecureNote -> "Secure note"
        isCreditCard -> (card ?: ItemCard()).summary
        isIdentity -> (identity ?: ItemIdentity()).summary
        isAPICredential -> (apiCredential ?: ItemAPICredential()).summary
        isAddress -> (address ?: ItemAddress()).summary
        isSoftwareLicense -> (softwareLicense ?: ItemSoftwareLicense()).summary
        isServer -> (server ?: ItemServer()).summary(username)
        isDatabase -> (database ?: ItemDatabase()).summary
        else -> "Login"
    }

/** The field the "Copy secret reference" button names, the one a script most likely wants, or null when there is none. */
val ItemDetail.referenceFieldName: String?
    get() = when {
        isSSHKey -> "public key"
        isSecureNote -> "notes".takeIf { notes.isNotEmpty() }
        isCreditCard -> CardFieldName.NUMBER.takeIf { !card?.number.isNullOrEmpty() }
        isIdentity || isAddress -> null
        isAPICredential -> APICredentialFieldName.SECRET.takeIf { !apiCredential?.secret.isNullOrEmpty() }
        isSoftwareLicense -> SoftwareLicenseFieldName.LICENSE_KEY.takeIf { !softwareLicense?.licenseKey.isNullOrEmpty() }
        else -> "password".takeIf { password.isNotEmpty() }
    }
