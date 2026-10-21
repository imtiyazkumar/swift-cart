package admin

import (
    "context"
    "errors"
    "strings"
)

func (r *pgRepo) ChangeRole(ctx context.Context, actor, userID, role, reason string) error {
    switch role {
    case "CUSTOMER", "MERCHANT_OWNER", "DELIVERY_PARTNER", "ADMIN":
    default:
        return errors.New("unsupported role")
    }
    if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
        return errors.New("a role-change reason of at most 2000 characters is required")
    }
    tx, err := r.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)
    if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894213)`); err != nil {
        return err
    }
    var previous string
    if err = tx.QueryRow(ctx, `SELECT role FROM public.users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&previous); err != nil {
        return errors.New("user not found")
    }
    if previous == "ADMIN" && role != "ADMIN" {
        var count int
        if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.users WHERE role='ADMIN' AND disabled_at IS NULL AND deleted_at IS NULL`).Scan(&count); err != nil {
            return err
        }
        if count <= 1 {
            return errors.New("cannot demote the last active administrator")
        }
    }
    if _, err = tx.Exec(ctx, `UPDATE public.users SET role=$2,updated_at=now() WHERE id=$1`, userID, role); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `UPDATE auth.refresh_sessions SET revoked_at=now() WHERE user_id=$1`, userID); err != nil {
        return err
    }
    if _, err = tx.Exec(ctx, `INSERT INTO public.admin_audit_logs(admin_id,action,target_type,target_id,details) VALUES($1,'USER_ROLE_CHANGED','user',$2,jsonb_build_object('previous',$3::text,'role',$4::text,'reason',$5::text))`, actor, userID, previous, role, reason); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
