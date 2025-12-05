package gatesentryproxy

import (
	"crypto/tls"
	"log"
	"net/http"
	"strconv"
	"time"

	// "crypto/x509"
	"sync"
)

type CertificateCache struct {
	lock        sync.RWMutex
	cache       map[certCacheKey]certCacheEntry
	TTL         time.Duration
	lastCleaned time.Time
}

type certCacheKey struct {
	name, addr string
}

type certCacheEntry struct {
	certificate tls.Certificate
	transport   http.RoundTripper
	added       time.Time
}

func (c *CertificateCache) Put(serverName, serverAddr string, cert tls.Certificate, transport http.RoundTripper) {

	c.lock.Lock()
	defer c.lock.Unlock()

	now := time.Now()
	if c.cache == nil {
		c.cache = make(map[certCacheKey]certCacheEntry)
		c.lastCleaned = now
	}

	// Lazy cleanup - only clean periodically to reduce overhead
	if now.Sub(c.lastCleaned) > c.TTL*2 {
		// Remove expired entries in batches to reduce lock contention
		count := 0
		maxClean := 100 // Clean max 100 entries at a time
		for k, v := range c.cache {
			if count >= maxClean {
				break
			}
			if now.Sub(v.added) > c.TTL {
				delete(c.cache, k)
				count++
			}
		}
		c.lastCleaned = now
	}

	c.cache[certCacheKey{
		name: serverName,
		addr: serverAddr,
	}] = certCacheEntry{
		certificate: cert,
		transport:   transport,
		added:       now,
	}
	if DebugLogging {
		log.Println("[CertificateCache] Size = " + strconv.Itoa(len(c.cache)))
	}
}

func (c *CertificateCache) Get(serverName, serverAddr string) (tls.Certificate, http.RoundTripper) {
	if DebugLogging {
		log.Println("[CertificateCache] Size = " + strconv.Itoa(len(c.cache)))
	}
	c.lock.RLock()
	defer c.lock.RUnlock()

	v, ok := c.cache[certCacheKey{
		name: serverName,
		addr: serverAddr,
	}]

	if !ok || time.Now().Sub(v.added) > c.TTL {
		return tls.Certificate{}, nil
	}

	return v.certificate, v.transport
}
