package gatesentryWebserverEndpoints

// func GetCertificateEndpoint(ctx iris.Context, data []byte) {
// 	ctx.ContentType("application/octet-stream")
// 	ctx.Header("content-disposition", "attachment; filename=\"certificate.der\"")
// 	ctx.Write(data)
// 	return
// }

// func GetHomeEndpoint(ctx iris.Context) {
// 	requestedId := ctx.Params().Get("id")
// 	html := `Welcome`
// 	html = gatesentry2responder.GetTemplate()
// 	content := `<h3>GateSentry Network Filters<br><span style="font-size:70%">Self Service Panel</span></h3><hr>`

// 	if requestedId == "" {
// 		// html = "Nothing requested"
// 		content += `
// 		<div>
// 			<strong>Why am I seeing this page?</strong>
// 			<p>It's because you have a GateSentry Internet filtering appliance installed on your
// 			network. You could do the following using this page:
// 			</p>
// 		</div>
// 		<div>
// 			<strong>I need to modify GateSentry's settings.</strong>
// 			<p>You can do that <a href="/">here</a>.
// 			</p>
// 		</div>
// 		<div>
// 			<strong>I need to install GateSentry's certificate for HTTPS filtering on my device.</strong>
// 			<p>Sure, you can download the certificate by clicking <a href="/home/certificate">here</a>.
// 			</p>
// 		</div>
// 		`
// 	} else {
// 		ctx.HTML("[GateSentry] Not found")
// 		return
// 	}

// 	html = strings.Replace(html, "_title_", "GateSentry Home", -1)
// 	html = strings.Replace(html, "_content_", content, -1)
// 	html = strings.Replace(html, "_mainstyle_", "margin-top:4%;", -1)
// 	html = strings.Replace(html, "_colorclass_", "mdl-color--blue", -1)
// 	html = strings.Replace(html, "_primarystyle_", string(gatesentryWebserverFrontend.GetStyles()), -1)
// 	ctx.Write([]byte(html))
// }
