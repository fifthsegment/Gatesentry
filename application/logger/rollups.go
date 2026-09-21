package gatesentry2logger

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/buntdb"
)

const (
	statsMinutePrefix = "stats:min:"
	statsHourPrefix   = "stats:hour:"
	rollupTTL         = 7 * 24 * time.Hour
	hourlyHostCap     = 20
	hostMapSoftCap    = 48
	userMapSoftCap    = 100
)

type minuteSnap struct {
	DNSAll         int            `json:"dns_all"`
	DNSBlocked     int            `json:"dns_blocked"`
	ProxyAllowed   int            `json:"proxy_allowed"`
	ProxyBlocked   int            `json:"proxy_blocked"`
	ProxySSLBump   int            `json:"proxy_ssl_bump"`
	ProxySSLDirect int            `json:"proxy_ssl_direct"`
	ProxyActions   map[string]int `json:"proxy_actions,omitempty"`
}

type hourUser struct {
	Total   int `json:"total"`
	Allowed int `json:"allowed"`
	Blocked int `json:"blocked"`
}

type hourSnap struct {
	DNSAll       map[string]int      `json:"dns_all,omitempty"`
	DNSBlocked   map[string]int      `json:"dns_blocked,omitempty"`
	ProxyAllowed map[string]int      `json:"proxy_allowed,omitempty"`
	ProxyBlocked map[string]int      `json:"proxy_blocked,omitempty"`
	Users        map[string]hourUser `json:"users,omitempty"`
}

// MinutePoint is one per-minute series sample for the stats page.
type MinutePoint struct {
	T              string         `json:"t"`
	DNSAll         int            `json:"dns_all"`
	DNSBlocked     int            `json:"dns_blocked"`
	ProxyAllowed   int            `json:"proxy_allowed"`
	ProxyBlocked   int            `json:"proxy_blocked"`
	ProxySSLBump   int            `json:"proxy_ssl_bump"`
	ProxySSLDirect int            `json:"proxy_ssl_direct"`
	ProxyActions   map[string]int `json:"proxy_actions,omitempty"`
}

// HostCount is a single host in an hourly top-N list.
type HostCount struct {
	Host  string `json:"host"`
	Count int    `json:"count"`
}

// HourHostSet is top-N hosts (and proxy users) for one local hour.
type HourHostSet struct {
	All          []HostCount     `json:"all"`
	Blocked      []HostCount     `json:"blocked"`
	ProxyAllowed []HostCount     `json:"proxy_allowed"`
	ProxyBlocked []HostCount     `json:"proxy_blocked"`
	Users        []HourUserCount `json:"users,omitempty"`
}

// HourUserCount is per-user proxy totals for one hour.
type HourUserCount struct {
	User    string `json:"user"`
	Total   int    `json:"total"`
	Allowed int    `json:"allowed"`
	Blocked int    `json:"blocked"`
}

// TrafficSeries is the compact 7-day payload for stats charts.
type TrafficSeries struct {
	Minutes     []MinutePoint          `json:"minutes"`
	HourlyHosts map[string]HourHostSet `json:"hourly_hosts"`
}

func (L *Log) initRollups() {
	L.minutes = make(map[string]*minuteSnap)
	L.hours = make(map[string]*hourSnap)
	L.dirtyMinutes = make(map[string]struct{})
	L.dirtyHours = make(map[string]struct{})
	if L.Database == nil {
		L.rollupsReady.Store(true)
		return
	}
	go L.persistLoop()
	go L.ensureRollups()
}

func (L *Log) persistLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-L.stopCh:
			return
		case <-ticker.C:
			L.persistDirty()
		}
	}
}

func (L *Log) ensureRollups() {
	if L == nil {
		return
	}
	L.rollupOnce.Do(func() {
		if L.Database == nil {
			L.rollupsReady.Store(true)
			return
		}
		if !L.hasStoredRollups() {
			L.rebuildFromLogs()
			L.persistDirty()
		}
		L.rollupsReady.Store(true)
	})
}

func (L *Log) hasStoredRollups() bool {
	found := false
	_ = L.Database.View(func(tx *buntdb.Tx) error {
		return tx.AscendGreaterOrEqual("", statsMinutePrefix, func(key, _ string) bool {
			if strings.HasPrefix(key, statsMinutePrefix) {
				found = true
				return false
			}
			return false
		})
	})
	return found
}

func (L *Log) rebuildFromLogs() {
	if L.Database == nil {
		return
	}
	now := time.Now().Unix()
	from := now - int64(rollupTTL/time.Second)
	fromS := strconv.FormatInt(from, 10)
	toS := strconv.FormatInt(now, 10)

	_ = L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+toS+`}`, `{"time":`+fromS+`}`, func(_, value string) bool {
			var e LogEntry
			if json.Unmarshal([]byte(value), &e) != nil {
				return true
			}
			L.applyRollup(e)
			return true
		})
	})
}

// Observe records an already-stored log entry in the stats rollups.
func (L *Log) Observe(e LogEntry) {
	L.applyRollup(e)
}

func (L *Log) applyRollup(e LogEntry) {
	if L == nil {
		return
	}
	if e.Type != "dns" && e.Type != "proxy" {
		return
	}
	ts := time.Unix(e.Time, 0).Local()
	minKey := ts.Format("2006-01-02T15:04")
	hourKey := ts.Format("2006-01-02T15")
	host := e.URL
	if e.Type == "proxy" {
		host = strings.ReplaceAll(host, "http://", "")
		host = strings.ReplaceAll(host, ":443", "")
	}

	L.rollupMu.Lock()
	defer L.rollupMu.Unlock()
	if L.minutes == nil {
		L.minutes = make(map[string]*minuteSnap)
		L.hours = make(map[string]*hourSnap)
		L.dirtyMinutes = make(map[string]struct{})
		L.dirtyHours = make(map[string]struct{})
	}

	m := L.minutes[minKey]
	if m == nil {
		m = &minuteSnap{}
		L.minutes[minKey] = m
	}
	h := L.hours[hourKey]
	if h == nil {
		h = &hourSnap{}
		L.hours[hourKey] = h
	}

	switch e.Type {
	case "dns":
		m.DNSAll++
		if h.DNSAll == nil {
			h.DNSAll = make(map[string]int)
		}
		bumpHost(h.DNSAll, host)
		if e.DNSResponseType == "blocked" {
			m.DNSBlocked++
			if h.DNSBlocked == nil {
				h.DNSBlocked = make(map[string]int)
			}
			bumpHost(h.DNSBlocked, host)
		}
	case "proxy":
		if m.ProxyActions == nil {
			m.ProxyActions = make(map[string]int)
		}
		m.ProxyActions[e.ProxyResponseType]++
		user := e.IP
		if user == "" {
			user = "anonymous"
		}
		if h.Users == nil {
			h.Users = make(map[string]hourUser)
		}
		u := h.Users[user]
		u.Total++
		if isProxyBlocked(e.ProxyResponseType) {
			m.ProxyBlocked++
			u.Blocked++
			if h.ProxyBlocked == nil {
				h.ProxyBlocked = make(map[string]int)
			}
			bumpHost(h.ProxyBlocked, host)
		} else if isProxyAllowed(e.ProxyResponseType) {
			m.ProxyAllowed++
			u.Allowed++
			if h.ProxyAllowed == nil {
				h.ProxyAllowed = make(map[string]int)
			}
			bumpHost(h.ProxyAllowed, host)
		}
		h.Users[user] = u
		pruneUsers(h.Users)
		switch e.ProxyResponseType {
		case "ssl-bump":
			m.ProxySSLBump++
		case "ssldirect":
			m.ProxySSLDirect++
		}
	}

	L.dirtyMinutes[minKey] = struct{}{}
	L.dirtyHours[hourKey] = struct{}{}
}

func isProxyAllowed(action string) bool {
	switch action {
	case "ssldirect", "ssl-bump", "filternone":
		return true
	default:
		return false
	}
}

func bumpHost(m map[string]int, host string) {
	if host == "" {
		host = "unknown"
	}
	m[host]++
	if len(m) <= hostMapSoftCap {
		return
	}
	keepTopN(m, hourlyHostCap)
}

func pruneUsers(m map[string]hourUser) {
	if len(m) <= userMapSoftCap {
		return
	}
	type kv struct {
		k string
		v hourUser
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v.Total > list[j].v.Total })
	for i := userMapSoftCap; i < len(list); i++ {
		delete(m, list[i].k)
	}
}

func keepTopN(m map[string]int, n int) {
	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	for i := n; i < len(list); i++ {
		delete(m, list[i].k)
	}
}

func (L *Log) persistDirty() {
	if L == nil || L.Database == nil {
		return
	}
	L.rollupMu.Lock()
	minKeys := make([]string, 0, len(L.dirtyMinutes))
	for k := range L.dirtyMinutes {
		minKeys = append(minKeys, k)
	}
	hourKeys := make([]string, 0, len(L.dirtyHours))
	for k := range L.dirtyHours {
		hourKeys = append(hourKeys, k)
	}
	minCopy := make(map[string]minuteSnap, len(minKeys))
	hourCopy := make(map[string]hourSnap, len(hourKeys))
	for _, k := range minKeys {
		if s := L.minutes[k]; s != nil {
			minCopy[k] = *s
			if s.ProxyActions != nil {
				m := make(map[string]int, len(s.ProxyActions))
				for ak, av := range s.ProxyActions {
					m[ak] = av
				}
				cp := minCopy[k]
				cp.ProxyActions = m
				minCopy[k] = cp
			}
		}
	}
	for _, k := range hourKeys {
		if s := L.hours[k]; s != nil {
			hourCopy[k] = cloneHour(*s)
		}
	}
	L.dirtyMinutes = make(map[string]struct{})
	L.dirtyHours = make(map[string]struct{})
	L.rollupMu.Unlock()

	if len(minCopy) == 0 && len(hourCopy) == 0 {
		return
	}
	_ = L.Database.Update(func(tx *buntdb.Tx) error {
		opt := &buntdb.SetOptions{Expires: true, TTL: rollupTTL}
		for k, s := range minCopy {
			b, err := json.Marshal(s)
			if err != nil {
				continue
			}
			_, _, _ = tx.Set(statsMinutePrefix+k, string(b), opt)
		}
		for k, s := range hourCopy {
			b, err := json.Marshal(s)
			if err != nil {
				continue
			}
			_, _, _ = tx.Set(statsHourPrefix+k, string(b), opt)
		}
		return nil
	})
}

func cloneHour(s hourSnap) hourSnap {
	out := hourSnap{
		DNSAll:       cloneIntMap(s.DNSAll),
		DNSBlocked:   cloneIntMap(s.DNSBlocked),
		ProxyAllowed: cloneIntMap(s.ProxyAllowed),
		ProxyBlocked: cloneIntMap(s.ProxyBlocked),
	}
	if s.Users != nil {
		out.Users = make(map[string]hourUser, len(s.Users))
		for k, v := range s.Users {
			out.Users[k] = v
		}
	}
	return out
}

func cloneIntMap(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetTrafficSeries returns per-minute counts and hourly top hosts for the window.
func (L *Log) GetTrafficSeries(fromSeconds int64) TrafficSeries {
	out := TrafficSeries{
		Minutes:     []MinutePoint{},
		HourlyHosts: map[string]HourHostSet{},
	}
	if L == nil {
		return out
	}
	L.ensureRollups()
	if fromSeconds <= 0 {
		fromSeconds = int64(rollupTTL / time.Second)
	}
	now := time.Now()
	from := now.Add(-time.Duration(fromSeconds) * time.Second)
	fromMin := from.Local().Format("2006-01-02T15:04")
	fromHour := from.Local().Format("2006-01-02T15")

	minDB := map[string]minuteSnap{}
	hourDB := map[string]hourSnap{}
	if L.Database != nil {
		_ = L.Database.View(func(tx *buntdb.Tx) error {
			_ = tx.AscendGreaterOrEqual("", statsMinutePrefix+fromMin, func(key, value string) bool {
				if !strings.HasPrefix(key, statsMinutePrefix) {
					return false
				}
				k := strings.TrimPrefix(key, statsMinutePrefix)
				if k < fromMin {
					return true
				}
				var s minuteSnap
				if json.Unmarshal([]byte(value), &s) != nil {
					return true
				}
				minDB[k] = s
				return true
			})
			_ = tx.AscendGreaterOrEqual("", statsHourPrefix+fromHour, func(key, value string) bool {
				if !strings.HasPrefix(key, statsHourPrefix) {
					return false
				}
				k := strings.TrimPrefix(key, statsHourPrefix)
				if k < fromHour {
					return true
				}
				var s hourSnap
				if json.Unmarshal([]byte(value), &s) != nil {
					return true
				}
				hourDB[k] = s
				return true
			})
			return nil
		})
	}

	L.rollupMu.Lock()
	for k, s := range L.minutes {
		if k >= fromMin && s != nil {
			minDB[k] = *s
			if s.ProxyActions != nil {
				cp := minDB[k]
				cp.ProxyActions = cloneIntMap(s.ProxyActions)
				minDB[k] = cp
			}
		}
	}
	for k, s := range L.hours {
		if k >= fromHour && s != nil {
			hourDB[k] = cloneHour(*s)
		}
	}
	L.rollupMu.Unlock()

	keys := make([]string, 0, len(minDB))
	for k := range minDB {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out.Minutes = make([]MinutePoint, 0, len(keys))
	for _, k := range keys {
		s := minDB[k]
		out.Minutes = append(out.Minutes, MinutePoint{
			T:              k,
			DNSAll:         s.DNSAll,
			DNSBlocked:     s.DNSBlocked,
			ProxyAllowed:   s.ProxyAllowed,
			ProxyBlocked:   s.ProxyBlocked,
			ProxySSLBump:   s.ProxySSLBump,
			ProxySSLDirect: s.ProxySSLDirect,
			ProxyActions:   s.ProxyActions,
		})
	}
	out.HourlyHosts = make(map[string]HourHostSet, len(hourDB))
	for k, s := range hourDB {
		out.HourlyHosts[k] = HourHostSet{
			All:          topHosts(s.DNSAll, hourlyHostCap),
			Blocked:      topHosts(s.DNSBlocked, hourlyHostCap),
			ProxyAllowed: topHosts(s.ProxyAllowed, hourlyHostCap),
			ProxyBlocked: topHosts(s.ProxyBlocked, hourlyHostCap),
			Users:        usersList(s.Users),
		}
	}
	return out
}

func topHosts(m map[string]int, n int) []HostCount {
	if len(m) == 0 {
		return []HostCount{}
	}
	list := make([]HostCount, 0, len(m))
	for h, c := range m {
		list = append(list, HostCount{Host: h, Count: c})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func usersList(m map[string]hourUser) []HourUserCount {
	if len(m) == 0 {
		return nil
	}
	list := make([]HourUserCount, 0, len(m))
	for u, c := range m {
		list = append(list, HourUserCount{
			User: u, Total: c.Total, Allowed: c.Allowed, Blocked: c.Blocked,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Total > list[j].Total })
	return list
}

// WaitRollupsForTest blocks until the startup backfill has finished.
func (L *Log) WaitRollupsForTest() {
	if L == nil {
		return
	}
	L.ensureRollups()
}
