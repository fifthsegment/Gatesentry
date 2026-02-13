package gatesentry2responder

// GetCssString returns minimal CSS for the GateSentry block page.
// This replaces the full Material Design Lite framework (132KB) with only
// the styles actually used by the block page template (~1.5KB).
func GetCssString() string {
	return `
*,*::before,*::after{box-sizing:border-box}
html,body{width:100%;height:100%;margin:0;padding:0;font-family:'Roboto',Helvetica,Arial,sans-serif;font-size:14px;font-weight:400;line-height:20px;color:rgba(0,0,0,.87)}
main{display:block}
img{vertical-align:middle}
h4{font-size:24px;font-weight:400;line-height:32px;margin:0 0 16px}
h5{font-size:20px;font-weight:500;line-height:1;margin:0 0 16px}
p{font-size:14px;font-weight:400;line-height:24px;margin:0 0 16px}
ul{font-size:14px;font-weight:400;line-height:24px}
.mdl-layout__container{position:relative;width:100%;height:100%}
.mdl-layout{width:100%;height:100%;display:flex;flex-direction:column;overflow-y:auto;overflow-x:hidden;position:relative}
.mdl-layout--fixed-header{}
.mdl-js-layout{}
.mdl-layout__content{flex-grow:1;position:relative;display:inline-block;overflow-y:auto;overflow-x:hidden}
.mdl-grid{display:flex;flex-flow:row wrap;margin:0 auto;align-items:stretch;padding:8px}
.mdl-cell{box-sizing:border-box;margin:8px}
.mdl-cell--2-col{width:calc(16.6666666667% - 16px)}
.mdl-cell--8-col{width:calc(66.6666666667% - 16px)}
.mdl-color--red{background-color:#f44336!important}
.mdl-color--white{background-color:#fff!important}
.mdl-color-text--grey-800{color:#424242!important}
.mdl-shadow--4dp{box-shadow:0 4px 5px 0 rgba(0,0,0,.14),0 1px 10px 0 rgba(0,0,0,.12),0 2px 4px -1px rgba(0,0,0,.2)}
@media (max-width:479px){
.mdl-cell{width:calc(100% - 16px)}
.mdl-cell--hide-phone{display:none!important}
}
@media (min-width:480px) and (max-width:839px){
.mdl-cell--hide-tablet{display:none!important}
.mdl-cell--2-col{width:calc(25% - 16px)}
.mdl-cell--8-col{width:calc(100% - 16px)}
}
`
}
