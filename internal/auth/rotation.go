package auth

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

// consumeRefresh binds rotated credentials to a fixed 30-day family lifetime.
// Reuse revokes descendants and commits that revocation before returning an error.
func consumeRefresh(ctx context.Context, tx pgx.Tx, raw, user string) (string, time.Time, error) {
	var family string
	var absolute, expiry time.Time
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT family_id,family_expires_at,expires_at,revoked_at FROM public.refresh_tokens WHERE token_hash=$1 AND user_id=$2 FOR UPDATE`, tokenHash(raw), user).Scan(&family, &absolute, &expiry, &revoked)
	if err != nil {
		return "", absolute, errors.New("refresh credential unavailable")
	}
	if revoked != nil || time.Now().After(expiry) || time.Now().After(absolute) {
		if _, err = tx.Exec(ctx, `UPDATE public.refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE family_id=$1`, family); err != nil {
			return "", absolute, err
		}
		if _, err = tx.Exec(ctx, `UPDATE public.users SET tokens_invalid_before=date_trunc('milliseconds',clock_timestamp()) WHERE id=$1`, user); err != nil {
			return "", absolute, err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", absolute, err
		}
		return "", absolute, errors.New("refresh credential replayed or expired")
	}
	if _, err = tx.Exec(ctx, `UPDATE public.refresh_tokens SET revoked_at=now() WHERE token_hash=$1`, tokenHash(raw)); err != nil {
		return "", absolute, err
	}
	return family, absolute, nil
}
