package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// PushTarget is a phone to send a push to: its device, and either its push relay handle or its Web Push subscription.
// PayloadKey, with a handle only, is the key an iPhone opens a push's details with.
type PushTarget struct {
	DeviceID   string
	Handle     string
	PayloadKey *PushPayloadKey
	WebPush    *WebPushSubscription
}

// PushPayloadKey is an iPhone's public key and authentication secret for push details, decoded.
type PushPayloadKey struct {
	P256DH []byte
	Auth   []byte
}

// WebPushSubscription is a Web Push endpoint with the phone's public key and authentication secret, decoded.
type WebPushSubscription struct {
	Endpoint string
	P256DH   []byte
	Auth     []byte
}

// SetDevicePush saves where a phone that is still signed in gets its pushes, target.Handle or target.WebPush, and its
// push kinds; for any other device it fails with ErrNotFound.
func (s *Store) SetDevicePush(ctx context.Context, accountID string, target PushTarget, allKind []string) error {
	handle, endpoint, p256dh, auth := target.columns()
	var payloadP256DH, payloadAuth []byte
	if target.PayloadKey != nil && target.WebPush == nil {
		payloadP256DH, payloadAuth = target.PayloadKey.P256DH, target.PayloadKey.Auth
	}
	commandTag, err := s.pool.Exec(ctx, `
		INSERT INTO device_push (device_id, handle, web_push_endpoint, web_push_p256dh, web_push_auth, payload_p256dh, payload_auth, kinds)
		SELECT id, $3, $4, $5, $6, $7, $8, $9 FROM devices WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL AND platform LIKE '%/mobile'
		ON CONFLICT (device_id) DO UPDATE SET handle = excluded.handle, web_push_endpoint = excluded.web_push_endpoint,
			web_push_p256dh = excluded.web_push_p256dh, web_push_auth = excluded.web_push_auth,
			payload_p256dh = excluded.payload_p256dh, payload_auth = excluded.payload_auth, kinds = excluded.kinds, updated_at = now()`,
		target.DeviceID, accountID, handle, endpoint, p256dh, auth, payloadP256DH, payloadAuth, allKind)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// columns returns the target's handle or Web Push subscription as the nullable columns of device_push.
func (target PushTarget) columns() (handle, endpoint *string, p256dh, auth []byte) {
	if target.WebPush != nil {
		return nil, &target.WebPush.Endpoint, target.WebPush.P256DH, target.WebPush.Auth
	}
	return &target.Handle, nil, nil, nil
}

func (s *Store) DeleteDevicePush(ctx context.Context, deviceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM device_push WHERE device_id = $1`, deviceID)
	return err
}

// DeleteStalePushTarget forgets a handle or Web Push endpoint that no longer delivers, unless the phone has since sent
// a new one.
func (s *Store) DeleteStalePushTarget(ctx context.Context, target PushTarget) error {
	handle, endpoint, _, _ := target.columns()
	_, err := s.pool.Exec(ctx, `
		DELETE FROM device_push WHERE device_id = $1 AND handle IS NOT DISTINCT FROM $2 AND web_push_endpoint IS NOT DISTINCT FROM $3`,
		target.DeviceID, handle, endpoint)
	return err
}

// ListPushTargets returns the account's signed-in devices that want pushes of the kind, except exceptDeviceID.
func (s *Store) ListPushTargets(ctx context.Context, accountID, kind, exceptDeviceID string) ([]PushTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, p.handle, p.web_push_endpoint, p.web_push_p256dh, p.web_push_auth, p.payload_p256dh, p.payload_auth
		FROM device_push p JOIN devices d ON d.id = p.device_id
		WHERE d.account_id = $1 AND d.revoked_at IS NULL AND $2 = ANY (p.kinds) AND d.id::text <> $3
		ORDER BY d.id`,
		accountID, kind, exceptDeviceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PushTarget, error) {
		var target PushTarget
		var handle, endpoint *string
		var p256dh, auth, payloadP256DH, payloadAuth []byte
		if err := row.Scan(&target.DeviceID, &handle, &endpoint, &p256dh, &auth, &payloadP256DH, &payloadAuth); err != nil {
			return target, err
		}
		if endpoint != nil {
			target.WebPush = &WebPushSubscription{Endpoint: *endpoint, P256DH: p256dh, Auth: auth}
		} else if handle != nil {
			target.Handle = *handle
			if payloadP256DH != nil {
				target.PayloadKey = &PushPayloadKey{P256DH: payloadP256DH, Auth: payloadAuth}
			}
		}
		return target, nil
	})
}
