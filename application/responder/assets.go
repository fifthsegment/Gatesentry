package gatesentry2responder

// GetCssString returns clean, modern CSS for the GateSentry block page.
func GetCssString() string {
	return `
*,*::before,*::after{box-sizing:border-box}
body{margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:14px;line-height:1.5;color:#333;background:#e53935;min-height:100vh;display:flex;align-items:center;justify-content:center}
.block-card{background:#fff;border-radius:12px;box-shadow:0 8px 32px rgba(0,0,0,.18);max-width:540px;width:90%;margin:32px auto;padding:40px 36px;text-align:center}
.block-card img{max-width:120px;margin:0 auto 20px}
.block-card h1{font-size:22px;font-weight:600;color:#333;margin:0 0 12px}
.block-card .msg{font-size:15px;color:#555;line-height:1.6;margin:0 0 8px}
.block-card .msg strong{color:#c62828}
.block-card .divider{width:60px;height:3px;background:#e53935;margin:16px auto;border-radius:2px}
.block-card .footer{font-size:12px;color:#999;margin-top:20px}
`
}
