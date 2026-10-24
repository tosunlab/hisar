package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// PushTarget is a phone to send a push to: its device and its push relay handle.
type PushTarget struct {
	DeviceID string
	Handle   string
}

// SetDevicePush saves the push relay handle and the push kinds of a phone that is still signed in; for any other device
// it fails with ErrNotFound.
func (s *Store) SetDevicePush(ctx context.Context, accountID, deviceID, handle string, allKind []string) error {
	commandTag, err := s.pool.Exec(ctx, `
		INSERT INTO device_push (device_id, handle, kinds)
		SELECT id, $3, $4 FROM devices WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL AND platform LIKE '%/mobile'
		ON CONFLICT (device_id) DO UPDATE SET handle = excluded.handle, kinds = excluded.kinds, updated_at = now()`,
		deviceID, accountID, handle, allKind)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteDevicePush(ctx context.Context, deviceID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM device_push WHERE device_id = $1`, deviceID)
	return err
}

// DeleteDevicePushHandle forgets a handle the push relay no longer delivers to, unless the phone has since sent a new
// one.
func (s *Store) DeleteDevicePushHandle(ctx context.Context, deviceID, handle string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM device_push WHERE device_id = $1 AND handle = $2`, deviceID, handle)
	return err
}

// ListPushTargets returns the account's signed-in devices that want pushes of the kind, except exceptDeviceID.
func (s *Store) ListPushTargets(ctx context.Context, accountID, kind, exceptDeviceID string) ([]PushTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, p.handle FROM device_push p JOIN devices d ON d.id = p.device_id
		WHERE d.account_id = $1 AND d.revoked_at IS NULL AND $2 = ANY (p.kinds) AND d.id::text <> $3
		ORDER BY d.id`,
		accountID, kind, exceptDeviceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PushTarget, error) {
		var target PushTarget
		err := row.Scan(&target.DeviceID, &target.Handle)
		return target, err
	})
}
