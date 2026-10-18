# Chrome Web Store submission

Hisar is published as an **unlisted** item: only people with the link can install it. Distribution decision: DESIGN §9.2.

## Packages

```sh
cd extension
npm ci && npm run build
npm run package:store -- --key-file ~/.hisar/chrome-web-store-key.pem   # first upload only
npm run package:store                                                  # every later upload
```

The store rejects a manifest with a `key` field, so `package:store` removes it. The first upload must contain the private key as `key.pem`; the store then keeps the ID `mmipiffoogblblocokimpemdopmdjghf`, which the desktop app's native host allows. Later uploads must not contain the key. Delete `hisar-extension-*-store-first-upload.zip` after uploading: it holds the private key.

## First submission

1. Create the developer account at https://chrome.google.com/webstore/devconsole (one-time registration fee) and verify the contact email.
2. **Add new item** and upload `hisar-extension-<version>-store-first-upload.zip`.
3. **Store listing:** the text below, the icon `dist/icons/icon-128.png` and the screenshots in `store/screenshots/` (1280×800).
4. **Privacy:** the single purpose, the permission justifications and the data usage answers below. Privacy policy URL: `https://api.tryhisar.com/privacy`, served by `hisar-server`.
5. **Distribution:** Visibility **Unlisted**, all regions.
6. Submit for review. After it is published, check that the item's ID is `mmipiffoogblblocokimpemdopmdjghf`, then share the store link with the people you invited.

## Store listing

- **Name:** Hisar
- **Summary** (132 characters at most): Fill and save logins from Hisar, the end-to-end encrypted password, SSH key and secrets manager for developers.
- **Category:** Tools
- **Language:** English
- **Description:**

  > Hisar is an end-to-end encrypted password, SSH key and secrets manager for developers. This extension connects your browser to the Hisar app on your computer.
  >
  > • Fill logins from the Hisar icon in login fields, from the toolbar or with Cmd/Ctrl+Shift+L
  > • Hisar fills a login only on the site it was saved for, never into frames from other sites
  > • Save new logins and update changed passwords as you sign in
  > • Generate strong passwords
  >
  > The extension holds no passwords and no keys. It talks only to the Hisar desktop app on the same computer, which must be installed and signed in to your Hisar account. Nothing is sent to the internet by the extension.

## Privacy practices

**Single purpose:** Fill, save and generate website logins using the Hisar password manager app installed on the same computer.

**Permission justifications:**

| Permission | Justification |
|---|---|
| `nativeMessaging` | The extension has no data of its own; it asks the Hisar desktop app on the same computer for matching logins, fills, saves and generated passwords. |
| `storage` | Stores the extension's pairing with the Hisar app, its two settings, the sites the user chose "Never for this site" for, and when each item was last filled (IDs and times only), to list recently used logins first. |
| `scripting` | Lists the frames of the current tab and fills a login into the page only after the user chooses it, respecting the same-site frame rule. |
| `activeTab` | Reads the address of the tab the user opened the popup or pressed the fill shortcut on, to find matching logins. |
| `tabs` | Finds the current tab's address for the popup and keeps save offers with their tab while the page navigates after signing in. |
| `contextMenus` | Adds "Show Hisar here", "Fill login" and "Fill a suggested password" to the menu of editable fields. |
| `offscreen` | Opens a hidden extension page whose only job is to put the current one-time code on the clipboard after the user fills a login that has one; a service worker cannot reach the clipboard itself. |
| `clipboardWrite` | That hidden page copies without a user gesture of its own, which Chrome allows only with this permission. Only the code of the login the user just filled is copied. |
| Host permission `<all_urls>` | The content script detects login forms on any site to show the Hisar icon and to offer saving a login after the user signs in; users keep logins for any website. |
| Remote code | No: all code is in the package. |

**Data usage:** the extension handles *authentication information* (usernames, passwords and the master password typed to unlock Hisar), *personally identifiable information* (names, emails, phone numbers and addresses of the user's identities), *financial and payment information* (the user's credit cards) and *website content* (the forms on the current page). For each:
- It is used only for the extension's single purpose.
- It is not sold, and not transferred to third parties.
- It is not used for creditworthiness or lending.
- It goes only to the Hisar app on the user's own computer.

Tick the three certification boxes.
