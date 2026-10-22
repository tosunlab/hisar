package app

import "errors"

func (store *memorySecretKeyStore) StoreRecoveryKey(id string, key []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.recoveryKeys == nil {
		store.recoveryKeys = map[string][]byte{}
	}
	store.recoveryKeys[id] = append([]byte(nil), key...)
	return nil
}

func (store *memorySecretKeyStore) LoadRecoveryKey(id string) ([]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key, exists := store.recoveryKeys[id]
	if !exists {
		return nil, errors.New("recovery key not found")
	}
	return append([]byte(nil), key...), nil
}

func (store *memorySecretKeyStore) DeleteRecoveryKey(id string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.recoveryKeys, id)
	return nil
}
