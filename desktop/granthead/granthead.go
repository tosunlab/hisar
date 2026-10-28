// Package granthead stores sealed grants' heads: on macOS in a login keychain item that only the creating Hisar program
// may read without a prompt, on Linux in a Secret Service item. Other platforms have no persistent store.
package granthead
