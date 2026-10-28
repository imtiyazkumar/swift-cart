package finance

import (
	"context"
	"go.uber.org/zap"
	"time"
)

func RunMaintenance(ctx context.Context, s *Service, log *zap.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, query := range []string{
				`UPDATE notifications.sms_outbox SET body='',status='EXPIRED' WHERE expires_at<now() AND status NOT IN ('DELIVERED','EXPIRED')`,
				`DELETE FROM auth.rate_windows WHERE window_start<now()-interval '2 days'`,
				`DELETE FROM public.otp_challenges WHERE expires_at<now()-interval '1 day'`,
				`UPDATE notifications.email_outbox SET body='' WHERE status='FAILED' AND created_at<now()-interval '1 day' AND subject IN ('Reset your Yember password','Verify your Yember email','Your Yember verification code','Your Yember delivery confirmation code')`,
			} {
				if _, err := s.DB.Exec(ctx, query); err != nil {
					log.Warn("operational cleanup failed", zap.Error(err))
				}
			}
		}
	}
}
