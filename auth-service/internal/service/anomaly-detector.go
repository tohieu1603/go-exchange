package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cryptox/auth-service/internal/model"
	"github.com/cryptox/auth-service/internal/repository"
	"github.com/cryptox/shared/mailer"
	"github.com/cryptox/shared/metrics"
	"github.com/redis/go-redis/v9"
)

// AnomalyDetector flags suspicious authentication patterns. Two heuristics:
//
//   1. Velocity   — N+ login.success events for the same user within window W
//                   (e.g. >5 logins in 60s). Caps brute-force success bursts.
//   2. Impossible-travel — two login.success events on different "geo regions"
//                   within Z seconds (e.g. /16 prefix change in <120s).
//                   Different /16 IPv4 in <2 min implies an attacker reusing
//                   the same credentials from a foreign network.
//
// Triggered post-login (sync or async — caller's choice). On detection:
//   • writes audit row action="anomaly.{velocity|geo}"
//   • sends email alert (asynchronous, non-blocking)
//
// Config via env (sane defaults baked in):
//   ANOMALY_VELOCITY_THRESHOLD   default 5
//   ANOMALY_VELOCITY_WINDOW_SEC  default 60
//   ANOMALY_GEO_WINDOW_SEC       default 120
type AnomalyDetector struct {
	rdb       *redis.Client
	auditRepo repository.AuditLogRepo
	audit     *AuditLogService
	es        *ESAnomalyClient // optional — nil disables ES-backed checks
	mail      mailer.Mailer
	users     repository.UserRepo
	velThresh int
	velWindow time.Duration
	geoWindow time.Duration
	// ES-backed thresholds — looser windows since ES holds longer history.
	esIPDistinctThresh int
	esIPDistinctWin    time.Duration
	esFailBurstThresh  int
	esFailBurstWin     time.Duration
}

func NewAnomalyDetector(
	rdb *redis.Client,
	auditRepo repository.AuditLogRepo,
	audit *AuditLogService,
	mail mailer.Mailer,
	users repository.UserRepo,
) *AnomalyDetector {
	if mail == nil {
		mail = mailer.NoopMailer{}
	}
	return &AnomalyDetector{
		rdb: rdb, auditRepo: auditRepo, audit: audit,
		mail: mail, users: users,
		velThresh: 5,
		velWindow: 60 * time.Second,
		geoWindow: 120 * time.Second,
		// ES-backed defaults (used only when SetES wired):
		esIPDistinctThresh: 4,
		esIPDistinctWin:    60 * time.Minute,
		esFailBurstThresh:  10,
		esFailBurstWin:     10 * time.Minute,
	}
}

// SetES wires the optional Elasticsearch query client. Without ES, only
// in-process Redis + DB heuristics run.
func (a *AnomalyDetector) SetES(es *ESAnomalyClient) { a.es = es }

// CheckLogin is called from Login on each successful authentication. Runs the
// two heuristics and emits alerts if either fires. Returns immediately on no-anomaly.
func (a *AnomalyDetector) CheckLogin(ctx context.Context, userID uint, email, ip string) {
	if a == nil || userID == 0 {
		return
	}
	now := time.Now()

	// 1. Velocity — Redis sliding-window counter.
	key := fmt.Sprintf("anomaly:vel:%d", userID)
	pipe := a.rdb.TxPipeline()
	cnt := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, a.velWindow)
	_, _ = pipe.Exec(ctx)
	if int(cnt.Val()) > a.velThresh {
		detail := fmt.Sprintf("velocity: %d successful logins in %s window", cnt.Val(), a.velWindow)
		a.audit.Failure(userID, email, "anomaly.velocity", ip, "", detail)
		metrics.AnomalyTotal.WithLabelValues("velocity").Inc()
		a.alertEmail(userID, email, "Phát hiện đăng nhập bất thường (velocity)", detail, now)
	}

	// 2. Impossible-travel — compare against last login.success in audit log.
	lastIP := a.lastSuccessIP(userID, ip)
	if lastIP != "" && !sameRegion(lastIP, ip) {
		detail := fmt.Sprintf("impossible_travel: previous IP %s, current IP %s", lastIP, ip)
		a.audit.Failure(userID, email, "anomaly.geo", ip, "", detail)
		metrics.AnomalyTotal.WithLabelValues("geo").Inc()
		a.alertEmail(userID, email, "Phát hiện đăng nhập bất thường (vị trí mới)", detail, now)
	}

	// 3. ES-backed wider-window checks. Skip if ES not configured.
	if a.es != nil {
		// 3a. Cross-IP burst — same user from many distinct IPs in 1h.
		if n := a.es.DistinctIPCountForUser(ctx, userID, a.esIPDistinctWin); n >= a.esIPDistinctThresh {
			detail := fmt.Sprintf("cross_ip_burst: %d distinct IPs in %s", n, a.esIPDistinctWin)
			a.audit.Failure(userID, email, "anomaly.cross_ip", ip, "", detail)
			metrics.AnomalyTotal.WithLabelValues("cross_ip").Inc()
			a.alertEmail(userID, email, "Phát hiện đăng nhập từ nhiều IP", detail, now)
		}
		// 3b. Failure-burst from this IP — distinct emails count.
		if n := a.es.FailureBurstFromIP(ctx, ip, a.esFailBurstWin); n >= a.esFailBurstThresh {
			detail := fmt.Sprintf("ip_failure_burst: %d distinct emails failed from this IP in %s",
				n, a.esFailBurstWin)
			// Use UserID=0 — this is an IP-level signal, not tied to the just-logged-in user.
			a.audit.Failure(0, "", "anomaly.stuffing", ip, "", detail)
			metrics.AnomalyTotal.WithLabelValues("stuffing").Inc()
		}
	}
}

// lastSuccessIP returns the IP of the most recent prior login.success row for
// this user, EXCLUDING the row matching `currentIP` (so we compare against
// the previous distinct IP, not the just-recorded one if projector raced).
func (a *AnomalyDetector) lastSuccessIP(userID uint, currentIP string) string {
	rows, _, err := a.auditRepo.ListByUser(userID, 1, 10)
	if err != nil {
		return ""
	}
	for _, r := range rows {
		if r.Action != model.AuditLoginSuccess || r.Outcome != model.AuditOutcomeSuccess {
			continue
		}
		if r.IP == "" || r.IP == currentIP {
			continue
		}
		// Only consider rows recent enough to matter.
		if time.Since(r.CreatedAt) > a.geoWindow {
			return ""
		}
		return r.IP
	}
	return ""
}

// sameRegion approximates "geo region" via IPv4 /16 prefix. Replace with
// MaxMind GeoIP2 country code in production for true geo distance.
//
// IPv6 falls back to /48 prefix.
// Loopback / private addresses always considered same region.
func sameRegion(a, b string) bool {
	if a == b || a == "" || b == "" {
		return true
	}
	if isLoopbackOrPrivate(a) || isLoopbackOrPrivate(b) {
		return true
	}
	if strings.Contains(a, ":") || strings.Contains(b, ":") {
		return prefix48(a) == prefix48(b)
	}
	return prefix16(a) == prefix16(b)
}

func prefix16(ip string) string {
	dots := 0
	for i := 0; i < len(ip); i++ {
		if ip[i] == '.' {
			dots++
			if dots == 2 {
				return ip[:i]
			}
		}
	}
	return ip
}

func prefix48(ip string) string {
	groups := 0
	for i := 0; i < len(ip); i++ {
		if ip[i] == ':' {
			groups++
			if groups == 3 {
				return ip[:i]
			}
		}
	}
	return ip
}

func isLoopbackOrPrivate(ip string) bool {
	switch {
	case ip == "127.0.0.1", ip == "::1":
		return true
	case strings.HasPrefix(ip, "10."),
		strings.HasPrefix(ip, "192.168."),
		strings.HasPrefix(ip, "172.16."), strings.HasPrefix(ip, "172.17."),
		strings.HasPrefix(ip, "172.18."), strings.HasPrefix(ip, "172.19."),
		strings.HasPrefix(ip, "172.2"), strings.HasPrefix(ip, "172.30."),
		strings.HasPrefix(ip, "172.31."):
		return true
	}
	return false
}

func (a *AnomalyDetector) alertEmail(userID uint, email, subject, detail string, when time.Time) {
	user, err := a.users.FindByID(userID)
	if err != nil {
		log.Printf("[anomaly] alert email lookup failed: %v", err)
		return
	}
	go func() {
		_ = a.mail.Send(email,
			subject+" — Micro-Exchange",
			anomalyEmailHTML(user.FullName, detail, when))
	}()
}

func anomalyEmailHTML(name, detail string, when time.Time) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Cảnh báo bảo mật — Hoạt động bất thường</h2>
<p>Xin chào %s,</p>
<p>Hệ thống vừa phát hiện hoạt động bất thường trên tài khoản:</p>
<table style="width:100%%;border-collapse:collapse;margin:16px 0">
  <tr><td style="padding:8px;background:#f3f4f6"><b>Thời gian</b></td><td style="padding:8px">%s</td></tr>
  <tr><td style="padding:8px;background:#f3f4f6"><b>Chi tiết</b></td><td style="padding:8px">%s</td></tr>
</table>
<p>Nếu không phải bạn, hãy đổi mật khẩu, bật 2FA và xem lại các phiên đăng nhập tại Bảo mật.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange Security</p>
</body></html>`, htmlEscape(name), when.Format("2006-01-02 15:04:05 -07:00"), htmlEscape(detail))
}

// htmlEscape — local minimal escaper to avoid importing the mailer package
// circularly. Keeps the file self-contained.
func htmlEscape(s string) string {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;", "\"", "&quot;")
	return r.Replace(s)
}
