package dispatch

import (
    "encoding/json"
    "github.com/imtiyazkumar/yember-core/config"
    "github.com/imtiyazkumar/yember-core/internal/auth"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v4"
    "go.uber.org/zap"
)

type Policy struct {MaxAttempts int `json:"max_attempts"`}

func RegisterSettings(api *echo.Group,pool *pgxpool.Pool,cfg *config.Config,log *zap.Logger) {
    group:=api.Group("/admin/settings",auth.JWTMiddleware(cfg.JWTSecret,log,pool),auth.RBACMiddleware("ADMIN"))
    group.GET("/dispatch",func(c echo.Context) error {
        var attempts int
        if err:=pool.QueryRow(c.Request().Context(),`SELECT COALESCE((SELECT (value->>'max_attempts')::int FROM public.platform_settings WHERE key='dispatch_policy'),0)`).Scan(&attempts);err!=nil{return echo.NewHTTPError(500,"could not load dispatch policy")}
        return c.JSON(200,Policy{MaxAttempts:attempts})
    })
    group.PUT("/dispatch",func(c echo.Context) error {
        var policy Policy
        if err:=c.Bind(&policy);err!=nil || policy.MaxAttempts<0 || policy.MaxAttempts>1000{return echo.NewHTTPError(400,"max_attempts must be between 0 and 1000; zero tries every eligible partner once")}
        tx,err:=pool.Begin(c.Request().Context());if err!=nil{return echo.NewHTTPError(500,"could not save dispatch policy")};defer tx.Rollback(c.Request().Context())
        data,_:=json.Marshal(policy);actor:=c.Get("userID")
        if _,err=tx.Exec(c.Request().Context(),`INSERT INTO public.platform_settings(key,value,updated_by) VALUES('dispatch_policy',$1,$2) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_by=EXCLUDED.updated_by,updated_at=now()`,data,actor);err!=nil{return echo.NewHTTPError(500,"could not save dispatch policy")}
        if _,err=tx.Exec(c.Request().Context(),`INSERT INTO public.admin_audit_logs(admin_id,action,target_type,details) VALUES($1,'DISPATCH_POLICY_UPDATED','platform_settings',$2)`,actor,data);err!=nil{return echo.NewHTTPError(500,"could not audit dispatch policy")}
        if err=tx.Commit(c.Request().Context());err!=nil{return echo.NewHTTPError(500,"could not save dispatch policy")}
        return c.JSON(200,policy)
    })
}
