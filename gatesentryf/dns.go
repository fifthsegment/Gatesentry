package gatesentryf

import (
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

var (
	blocklists = []string{
		"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		"https://easylist.to/easylist/easylist.txt",
		"https://raw.githubusercontent.com/anudeepND/blacklist/master/adservers.txt",
		"https://v.firebog.net/hosts/AdguardDNS.txt",
		"https://raw.githubusercontent.com/PolishFiltersTeam/KADhosts/master/KADhosts.txt",
		"https://raw.githubusercontent.com/FadeMind/hosts.extras/master/add.Spam/hosts",
		"https://v.firebog.net/hosts/static/w3kbl.txt",
		"https://adaway.org/hosts.txt",
		"https://v.firebog.net/hosts/RPiList-Phishing.txt",
		"https://v.firebog.net/hosts/RPiList-Malware.txt",
		"https://gitlab.com/quidsup/notrack-blocklists/raw/master/notrack-malware.txt",
		"https://pgl.yoyo.org/adservers/serverlist.php?hostformat=hosts&showintro=0&mimetype=plaintext",
		"https://bitbucket.org/ethanr/dns-blacklists/raw/8575c9f96e5b4a1308f2f12394abd86d0927a4a0/bad_lists/Mandiant_APT1_Report_Appendix_D.txt",
		// Add more blocklist URLs here
	}
)

func StartDNSServer(baseDir string, logger *gatesentry2logger.Log) {
	gatesentryDnsServer.StartDNSServer(baseDir, logger, blocklists)
}
