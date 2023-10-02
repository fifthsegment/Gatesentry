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

	if now.Sub(c.lastCleaned) > c.TTL {
		// Remove expired entries.
		for k, v := range c.cache {
			if now.Sub(v.added) > c.TTL {
				delete(c.cache, k)
			}
		}
	}

	c.cache[certCacheKey{
		name: serverName,
		addr: serverAddr,
	}] = certCacheEntry{
		certificate: cert,
		transport:   transport,
		added:       now,
	}
	log.Println("[CertificateCache] Size = " + strconv.Itoa(len(c.cache)))
}

func (c *CertificateCache) Get(serverName, serverAddr string) (tls.Certificate, http.RoundTripper) {
	log.Println("[CertificateCache] Size = " + strconv.Itoa(len(c.cache)))
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
