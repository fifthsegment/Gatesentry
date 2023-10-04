package gatesentry2responder

import (
	"strconv"
	"strings"

	gatesentry2frontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
)

type GSFilterResponder struct {
	Blocked bool
	Score   int
	Reasons []string
}

func GetTemplate() string {
	templ := `<html>
				<head>
				<meta name="viewport" content="width=device-width, initial-scale=1">
				<title>_title_</title>
				<style>
				 _primarystyle_
				</style>
				</head>
				<body>
					<div class="mdl-layout__container ">
					<div style="width:100%;" class=" mdl-layout mdl-layout--fixed-header mdl-js-layout _colorclass_ is-upgraded" >
					 <main style="width:100%; _mainstyle_" class="mdl-layout__content ">
				        <div class="mdl-grid" >
				          <div class="mdl-cell mdl-cell--2-col mdl-cell--hide-tablet mdl-cell--hide-phone"></div>
				          <div style="padding:20px; " class=" mdl-color--white mdl-shadow--4dp content mdl-color-text--grey-800 mdl-cell mdl-cell--8-col">
			            
								_content_

				          </div>
				        </div>
		
				      </main>
				      </div>
				    </div>
				</body>
			</html>`
	return templ
}

func GetBlockImage() string {
	image := `data:image/png;base64,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`
	return image
}

func BuildGeneralResponsePage(Reasons []string, Score int) string {
	content := "<p ><h4 style='text-align:center'>GateSentry Web Filter</h4></p>"
	content += "<p style='text-align:center'><img src='" + GetBlockImage() + "'></p>"
	KeywordsFound := "<h5 style='text-align:center'>"
	for i := 0; i < len(Reasons); i++ {
		KeywordsFound += "<span>" + Reasons[i] + "</span>"
	}
	KeywordsFound += "</h5>"
	content += KeywordsFound
	templ := GetTemplate()
	templ = strings.Replace(templ, "_title_", "Blocked", -1)
	templ = strings.Replace(templ, "_content_", content, -1)
	templ = strings.Replace(templ, "_mainstyle_", "margin-top:7% ", -1)
	templ = strings.Replace(templ, "_colorclass_", "mdl-color--red", -1)
	templ = strings.Replace(templ, "_primarystyle_", string(gatesentry2frontend.GetStyles()), -1)
	return templ
}

func BuildResponsePage(Reasons []string, Score int) string {
	// KeywordsFound := "";
	// for i := 0; i < len(Reasons); i++ {
	// 	KeywordsFound += "<li><b>"+Reasons[i]+"</b></li>"
	// }
	content := "<p ><h4 style='text-align:center'>GateSentry Web Filter</h4></p>"
	content += "<p style='text-align:center'><img src='" + GetBlockImage() + "'></p>"
	content += `<p>The page you requested has been blocked because it generated a score of
					<u>` + strconv.Itoa(Score) + `</u> which is above the viewing limits on this network.</p>`
	content += `<p>Reason(s) this page was blocked was presence of the following:</p>`
	KeywordsFound := "<ul >"
	for i := 0; i < len(Reasons); i++ {
		KeywordsFound += "<li><strong>" + Reasons[i] + "</strong></li>"
	}
	KeywordsFound += "</ul>"
	content += KeywordsFound
	templ := GetTemplate()
	templ = strings.Replace(templ, "_title_", "Blocked", -1)
	templ = strings.Replace(templ, "_content_", content, -1)
	templ = strings.Replace(templ, "_mainstyle_", "margin-top:7% ", -1)
	templ = strings.Replace(templ, "_colorclass_", "mdl-color--red", -1)
	return templ
	// data, err := gatesentry2webserver.Asset("app/material.css")
	// if err != nil {
	//     // Asset was not found.
	// }
	// jsdata, err := gatesentry2webserver.Asset("app/material.js")
	// if err != nil {
	//     // Asset was not found.
	// }
	// return `<html>
	// 			<title>Blocked</title>
	// 			<style>
	// 				`+string(data)+`
	// 			</style>
	// 			<body>
	// 				<div class="mdl-layout__container ">
	// 				<div style="width:100%;" class=" mdl-layout mdl-layout--fixed-header mdl-js-layout mdl-color--red is-upgraded" >
	// 				 <main style="width:100%; margin-top:15%;" class="mdl-layout__content ">
	// 			        <div class="mdl-grid" >
	// 			          <div class="mdl-cell mdl-cell--2-col mdl-cell--hide-tablet mdl-cell--hide-phone"></div>
	// 			          <div style="padding:20px; " class=" mdl-color--white mdl-shadow--4dp content mdl-color-text--grey-800 mdl-cell mdl-cell--8-col">

	// 			              The page you requested has been blocked because it generated a score of
	// 					<u>` +strconv.Itoa( Score) +`</u> which is above the viewing limits on this network.
	// 					<p>
	// 						Reasons this page was blocked was presence of the following:
	// 						<ul>
	// 						`+ KeywordsFound+ `
	// 						</ul>
	// 					</p>

	// 			          </div>
	// 			        </div>

	// 			      </main>
	// 			      </div>
	// 			    </div>
	// 			</body>
	// 			<script>
	// 				`+string(jsdata)+`
	// 			</script>

	// 		</html>`
}
