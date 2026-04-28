package mailer

import "fmt"

// Standard HTML templates used across services. Kept minimal — substitute with
// a real template engine if requirements grow.

func VerifyEmailHTML(name, code string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Xác thực email</h2>
<p>Xin chào %s,</p>
<p>Mã xác thực email của bạn là:</p>
<p style="font-size:28px;letter-spacing:8px;font-weight:bold;background:#f3f4f6;padding:16px;text-align:center;border-radius:8px">%s</p>
<p>Mã có hiệu lực trong 10 phút.</p>
<p>Nếu bạn không yêu cầu, vui lòng bỏ qua email này.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange</p>
</body></html>`, escape(name), escape(code))
}

func WithdrawalApprovedHTML(name string, amount float64, currency string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Rút tiền đã được phê duyệt</h2>
<p>Xin chào %s,</p>
<p>Yêu cầu rút <b>%.2f %s</b> của bạn đã được duyệt và đang được xử lý.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange</p>
</body></html>`, escape(name), amount, escape(currency))
}

func DepositConfirmedHTML(name string, amount float64, currency string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Nạp tiền thành công</h2>
<p>Xin chào %s,</p>
<p>Bạn đã nạp thành công <b>%.2f %s</b> vào ví. Vui lòng kiểm tra số dư.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange</p>
</body></html>`, escape(name), amount, escape(currency))
}

func NewDeviceLoginHTML(name, ip, userAgent, when string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Đăng nhập từ thiết bị mới</h2>
<p>Xin chào %s,</p>
<p>Chúng tôi vừa phát hiện đăng nhập tài khoản của bạn từ một thiết bị chưa từng sử dụng:</p>
<table style="width:100%%;border-collapse:collapse;margin:16px 0">
  <tr><td style="padding:8px;background:#f3f4f6"><b>Thời gian</b></td><td style="padding:8px">%s</td></tr>
  <tr><td style="padding:8px;background:#f3f4f6"><b>Địa chỉ IP</b></td><td style="padding:8px">%s</td></tr>
  <tr><td style="padding:8px;background:#f3f4f6"><b>Thiết bị</b></td><td style="padding:8px">%s</td></tr>
</table>
<p>Nếu là <b>bạn</b>, bạn có thể bỏ qua email này.</p>
<p>Nếu <b>không phải bạn</b>: hãy đổi mật khẩu ngay lập tức và bật xác thực hai yếu tố (2FA) tại trang Bảo mật.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange Security</p>
</body></html>`, escape(name), escape(when), escape(ip), escape(userAgent))
}

func PasswordChangedHTML(name string) string {
	return fmt.Sprintf(`<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px">
<h2>Mật khẩu đã thay đổi</h2>
<p>Xin chào %s,</p>
<p>Mật khẩu của bạn vừa được thay đổi và toàn bộ phiên đăng nhập đã được đăng xuất.</p>
<p>Nếu không phải bạn, vui lòng liên hệ hỗ trợ ngay lập tức.</p>
<p style="color:#777;font-size:12px">— Micro-Exchange</p>
</body></html>`, escape(name))
}

// escape — minimal HTML escaping for common chars in user-supplied fields.
func escape(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '<':
			out = append(out, []rune("&lt;")...)
		case '>':
			out = append(out, []rune("&gt;")...)
		case '&':
			out = append(out, []rune("&amp;")...)
		case '"':
			out = append(out, []rune("&quot;")...)
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
