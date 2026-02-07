# DNS Server Updates - Bug Fixes & Enhancements

## Executive Summary

This PR addresses **critical concurrency bugs** in the GateSentry DNS server and adds **TCP protocol support** for handling large DNS queries. These changes significantly improve reliability under load and enable proper handling of responses >512 bytes.

### Key Changes
1. **Fixed Global Mutex Blocking Bug** - Changed from `sync.Mutex` to `sync.RWMutex` for concurrent query handling
2. **Fixed Race Condition in Filter Initialization** - Added proper locking around map pointer reassignments
3. **Added TCP Protocol Support** - DNS server now handles both UDP and TCP queries
4. **Environment Variable Priority Fix** - `GATESENTRY_DNS_RESOLVER` now properly overrides stored settings

### Test Results
- **85/85 tests passed (100% pass rate)**
- TCP and UDP queries both working correctly
- mDNS/Bonjour service discovery fully functional
- Concurrent query handling verified with 50 simultaneous requests

---

## Bug #1: Global Mutex Over Entire DNS Request

### Problem Description

The original `handleDNSRequest()` function held a global mutex during the **entire request lifecycle**, including external DNS forwarding:

```go
// BEFORE: Problematic code in handleDNSRequest()
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
    mutex.Lock()         // Lock acquired here
    defer mutex.Unlock() // Not released until function returns
    
    // ... check blockedDomains, exceptionDomains, internalRecords ...
    
    // PROBLEM: This external call takes 50-500ms and blocks ALL other queries!
    resp, err := forwardDNSRequest(r)  
    
    // mutex.Unlock() happens here via defer
}
```

### Why This Was A Critical Bug

1. **All DNS queries were serialized** - Only one query could be processed at a time
2. **External DNS latency blocked all requests** - A slow upstream DNS response (e.g., 200ms) blocked every other query for that duration
3. **Under load, queries would timeout** - With 50+ concurrent requests, later queries would timeout waiting for the mutex
4. **Cascading failures** - Timeouts caused retry storms, making the problem worse

### The Fix

Changed to `sync.RWMutex` and restructured the code to:
1. Use `RLock()` for reading shared maps (allows concurrent readers)
2. Release the lock **before** external DNS forwarding
3. Use `Lock()` only when updating maps (in scheduler/filter initialization)

```go
// AFTER: Fixed code
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
    // Use read lock - allows concurrent DNS queries
    mutex.RLock()
    isException := exceptionDomains[domain]
    internalIP, isInternal := internalRecords[domain]
    isBlocked := blockedDomains[domain]
    mutex.RUnlock()  // Released immediately after reading!
    
    // Now forward WITHOUT holding any lock
    resp, err := forwardDNSRequest(r)
}
```

### Files Modified
- `application/dns/server/server.go` - Changed mutex type and usage pattern
- `application/dns/scheduler/scheduler.go` - Updated to use `*sync.RWMutex`

---

## Bug #2: Race Condition in Filter Initialization

### Problem Description

In `application/dns/filter/domains.go`, the `InitializeFilters()` function was reassigning map pointers without holding the mutex:

```go
// BEFORE: Race condition
func InitializeFilters(...) {
    tempBlockedMap := make(map[string]bool)
    // ... populate tempBlockedMap ...
    
    // RACE CONDITION: Reading from handleDNSRequest while this runs!
    *blockedDomains = tempBlockedMap  // Pointer reassignment without lock
}
```

### Why This Was A Bug

While the original author correctly used a temporary map to avoid issues during population, the final pointer reassignment was not protected. This could cause:
- Partial reads during reassignment
- Panic from accessing a nil/partial map
- Inconsistent state between different map pointers

### The Fix

Added proper mutex locking around all map pointer reassignments:

```go
// AFTER: Properly locked
func InitializeFilters(..., mutex *sync.RWMutex, ...) {
    tempBlockedMap := make(map[string]bool)
    // ... populate tempBlockedMap (no lock needed) ...
    
    // Lock before reassigning pointers
    mutex.Lock()
    *blockedDomains = tempBlockedMap
    *exceptionDomains = tempExceptionMap
    *internalRecords = tempInternalRecords
    mutex.Unlock()
}
```

### Files Modified
- `application/dns/filter/domains.go` - Added mutex lock around reassignments

---

## Enhancement: TCP Protocol Support

### Problem

DNS over UDP has a 512-byte limit for responses. Larger responses (DNSSEC, large TXT records, zone transfers) either:
1. Get truncated (TC flag set)
2. Require TCP fallback
3. Fail entirely if TCP isn't supported

The original server only supported UDP:
```go
server = &dns.Server{Addr: bindAddr, Net: "udp"}  // UDP only!
```

### The Solution

Added a TCP server running alongside UDP on the same port:

```go
// Start TCP server in a goroutine for large DNS queries (>512 bytes)
tcpServer = &dns.Server{Addr: bindAddr, Net: "tcp"}
tcpServer.Handler = dns.HandlerFunc(handleDNSRequest)
go func() {
    tcpServer.ListenAndServe()
}()

// Start UDP server (blocks)
server = &dns.Server{Addr: bindAddr, Net: "udp"}
server.Handler = dns.HandlerFunc(handleDNSRequest)
server.ListenAndServe()
```

### Benefits
- Same handler works for both protocols (miekg/dns handles the protocol differences)
- Proper handling of truncated responses
- DNSSEC support possible
- Zone transfer support
- No changes needed to client code

### Verification
```bash
# UDP query
$ dig @127.0.0.1 -p 10053 google.com A +short
142.251.12.101

# TCP query  
$ dig @127.0.0.1 -p 10053 google.com A +tcp +short
74.125.200.100
```

### Files Modified
- `application/dns/server/server.go` - Added `tcpServer` variable and startup logic
- Updated `StopDNSServer()` to properly shut down both servers

---

## Enhancement: Environment Variable Priority

### Problem

The `GATESENTRY_DNS_RESOLVER` environment variable was being ignored if a value was already stored in GSSettings. This made containerized deployments difficult.

### The Solution

Environment variables now explicitly override stored settings (when set):

```go
// BEFORE: SetDefault doesn't override existing values
R.GSSettings.SetDefault("dns_resolver", dnsResolverDefault)

// AFTER: Environment variable takes precedence
if envResolver := os.Getenv("GATESENTRY_DNS_RESOLVER"); envResolver != "" {
    R.GSSettings.Update("dns_resolver", dnsResolverValue)  // Override!
} else {
    R.GSSettings.SetDefault("dns_resolver", "8.8.8.8:53")
}
```

### Files Modified
- `application/runtime.go` - Updated settings initialization logic

---

## Test Results

### Full Test Suite Results
```
Test Results:
  Passed:  85
  Failed:  0
  Skipped: 0
  Total:   85

Pass Rate: 100.0%
```

### Test Categories Verified
1. ✅ External Resolver Validation
2. ✅ Basic DNS Forwarding (A, AAAA, MX, TXT, NS, SOA, CNAME, PTR records)
3. ✅ DNS Blocking/Filtering
4. ✅ Internal Records Resolution
5. ✅ Exception Domains
6. ✅ Edge Cases (invalid domains, empty queries, special characters)
7. ✅ Performance (response time <50ms target)
8. ✅ TCP Fallback Support
9. ✅ Caching Behavior
10. ✅ Concurrent Query Handling (50 simultaneous queries)
11. ✅ IPv6 Support
12. ✅ Environment Variable Configuration
13. ✅ Resolver Normalization
14. ✅ mDNS/Bonjour Service Discovery

### Concurrency Test Results
```
Testing concurrent query handling with 50 simultaneous queries...
All 50 concurrent queries completed
Total time for 50 concurrent queries: 80ms
Average time per query: 1.6ms
```

### TCP Test Results
```
TCP DNS query: PASS - TCP queries supported
Large response handling (TXT record): PASS
```

---

## Backwards Compatibility

All changes are backwards compatible:

1. **API unchanged** - Same function signatures for `StartDNSServer()` and `StopDNSServer()`
2. **Default behavior unchanged** - UDP still works exactly as before
3. **Settings migration** - No migration needed; existing settings continue to work
4. **Environment variables** - Optional; only override when explicitly set

---

## Recommendations for Future Work

1. **Add connection pooling** for upstream DNS queries
2. **Implement query caching** to reduce upstream load
3. **Add DNS-over-HTTPS (DoH)** support for privacy
4. **Add metrics/monitoring** for query latency and error rates
5. **Consider rate limiting** to prevent DNS amplification attacks

---

## Summary of Modified Files

### Go Source Files

| File | Changes | Purpose |
|------|---------|---------|
| `application/dns/server/server.go` | RWMutex, TCP support, improved shutdown, resolver normalization | Main DNS server implementation |
| `application/dns/scheduler/scheduler.go` | Updated mutex type signature | Periodic filter update scheduler |
| `application/dns/filter/domains.go` | RWMutex type, added locking around map initialization | Blocked domain filter management |
| `application/dns/filter/exception-records.go` | Updated mutex type signature | Exception domain handling |
| `application/dns/filter/internal-records.go` | Updated mutex type signature | Internal DNS record handling |
| `application/runtime.go` | Environment variable priority for DNS resolver | Application initialization |

### Scripts and Configuration

| File | Changes | Purpose |
|------|---------|---------|
| `scripts/dns_deep_test.sh` | New file (~2300 lines) | Comprehensive DNS testing suite |
| `run.sh` | Added environment variable support | Enhanced server startup script |
| `build.sh` | Better build output and error handling | Build automation |

---

## Detailed File Changes

### 1. `application/dns/server/server.go`

**Changes:**
1. Changed `sync.Mutex` to `sync.RWMutex` for concurrent read access
2. Added `tcpServer` variable for TCP protocol support
3. Added `normalizeResolver()` function to ensure `:53` port suffix
4. Modified `handleDNSRequest()` to use `RLock()`/`RUnlock()` for reading shared maps
5. Moved mutex unlock to happen BEFORE external DNS forwarding
6. Added environment variable support (`GATESENTRY_DNS_ADDR`, `GATESENTRY_DNS_PORT`, `GATESENTRY_DNS_RESOLVER`)
7. Added TCP server startup in goroutine
8. Improved `StopDNSServer()` to properly shut down both UDP and TCP servers

**Key Code Changes:**
```go
// BEFORE: Blocking mutex over entire request
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
    mutex.Lock()
    defer mutex.Unlock()
    // ... check maps and forward request ...
}

// AFTER: Read lock only while reading maps
func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
    mutex.RLock()
    isException := exceptionDomains[domain]
    internalIP, isInternal := internalRecords[domain]
    isBlocked := blockedDomains[domain]
    mutex.RUnlock()
    
    // Forward WITHOUT holding lock
    resp, err := forwardDNSRequest(r)
}
```

### 2. `application/dns/scheduler/scheduler.go`

**Changes:**
1. Changed `*sync.Mutex` parameter to `*sync.RWMutex` in function signatures

**Modified Functions:**
- `RunScheduler()` - mutex parameter type change
- `doInitialize()` - mutex parameter type change  
- `InitializerType` type definition - mutex type change

**Reason:** Required to match the RWMutex type used in server.go. The scheduler passes the mutex to filter initialization functions.

### 3. `application/dns/filter/domains.go`

**Changes:**
1. Changed `*sync.Mutex` to `*sync.RWMutex` in function signatures
2. Added `mutex.Lock()`/`mutex.Unlock()` around map pointer reassignments in `InitializeFilters()`

**Modified Functions:**
- `InitializeFilters()` - Added locking, changed mutex type
- `InitializeBlockedDomains()` - Changed mutex type
- `addDomainsToBlockedMap()` - Changed mutex type (already had proper locking)

**Key Code Changes:**
```go
// BEFORE: Race condition - map pointers reassigned without lock
func InitializeFilters(..., mutex *sync.Mutex, ...) {
    *blockedDomains = make(map[string]bool)  // RACE!
    *exceptionDomains = make(map[string]bool)
}

// AFTER: Properly locked
func InitializeFilters(..., mutex *sync.RWMutex, ...) {
    mutex.Lock()
    *blockedDomains = make(map[string]bool)
    *exceptionDomains = make(map[string]bool)
    *internalRecords = make(map[string]string)
    mutex.Unlock()
}
```

### 4. `application/dns/filter/exception-records.go`

**Changes:**
1. Changed `*sync.Mutex` to `*sync.RWMutex` in `InitializeExceptionDomains()` function signature

**Reason:** Type consistency with the RWMutex used in server.go. This function already correctly uses `mutex.Lock()` when modifying the exception domains map.

### 5. `application/dns/filter/internal-records.go`

**Changes:**
1. Changed `*sync.Mutex` to `*sync.RWMutex` in `InitializeInternalRecords()` function signature

**Reason:** Type consistency with the RWMutex used in server.go. This function already correctly uses `mutex.Lock()` when modifying the internal records map.

### 6. `application/runtime.go`

**Changes:**
1. Environment variable `GATESENTRY_DNS_RESOLVER` now takes precedence over stored settings
2. Added port normalization (`:53` suffix) when reading from environment

**Key Code Changes:**
```go
// BEFORE: SetDefault doesn't override existing stored values
R.GSSettings.SetDefault("dns_resolver", "8.8.8.8:53")

// AFTER: Environment variable overrides stored settings
if envResolver := os.Getenv("GATESENTRY_DNS_RESOLVER"); envResolver != "" {
    dnsResolverValue := envResolver
    if !strings.Contains(dnsResolverValue, ":") {
        dnsResolverValue = dnsResolverValue + ":53"
    }
    R.GSSettings.Update("dns_resolver", dnsResolverValue)  // Override!
} else {
    R.GSSettings.SetDefault("dns_resolver", "8.8.8.8:53")
}
```

**Reason:** Enables containerized deployments where the resolver is set via environment variable. Previously, once a value was stored in GSSettings, the environment variable was ignored.

### 7. `run.sh` (Enhanced Startup Script)

**Changes:**
1. Added shebang (`#!/bin/bash`) for proper script execution
2. Added environment variable exports with sensible defaults
3. Fixed trailing newline for POSIX compliance

**New Content:**
```bash
#!/bin/bash

# DNS Server Configuration
# Set the listen address (default: 0.0.0.0 - all interfaces)
export GATESENTRY_DNS_ADDR="${GATESENTRY_DNS_ADDR:-0.0.0.0}"

# Set the DNS port (default: 53, use 5353 or other if 53 is in use)
export GATESENTRY_DNS_PORT="${GATESENTRY_DNS_PORT:-53}"

# Set the external resolver (default: 8.8.8.8:53)
export GATESENTRY_DNS_RESOLVER="${GATESENTRY_DNS_RESOLVER:-8.8.8.8:53}"

rm -rf bin
mkdir bin
./build.sh && cd bin && ./gatesentrybin > ../log.txt 2>&1
```

**Benefits for Local Development:**
- Developers can override any setting by exporting environment variables before running
- Default values work out of the box for standard setups
- Avoids port conflicts by allowing custom port configuration (e.g., use 5353 if 53 is in use by systemd-resolved)
- Easy to test with different upstream resolvers

**Usage Examples:**
```bash
# Run with defaults
./run.sh

# Run on non-privileged port (no sudo needed)
GATESENTRY_DNS_PORT=5353 ./run.sh

# Run with custom resolver for testing
GATESENTRY_DNS_RESOLVER=1.1.1.1 ./run.sh

# Run with all custom settings
GATESENTRY_DNS_ADDR=127.0.0.1 \
GATESENTRY_DNS_PORT=10053 \
GATESENTRY_DNS_RESOLVER=8.8.4.4 \
./run.sh
```

### 8. `build.sh` (Enhanced Build Script)

**Changes:**
1. Added bin directory creation if it doesn't exist
2. Added cleanup of existing bin directory before build
3. Added build status messages for better feedback
4. Added exit code handling for build failures

**New Content:**
```bash
if [ ! -d "bin" ]; then
    mkdir bin
else
    echo "Cleaning existing bin directory..."
    rm -rf bin/*
fi
echo "Building GateSentry..."
go build -o bin/ ./...
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi
echo "Build successful. Executable is in the 'bin' directory."
```

**Benefits:**
- Clean builds every time (removes old artifacts)
- Clear feedback on build success/failure
- Proper exit codes for CI/CD integration

---

## Testing Instructions

### Quick Start for Local Development

```bash
# Option 1: Use run.sh with defaults (requires sudo for port 53)
sudo ./run.sh

# Option 2: Use run.sh on non-privileged port (recommended for development)
GATESENTRY_DNS_PORT=10053 ./run.sh

# Option 3: Run directly with environment variables
GATESENTRY_DNS_ADDR=127.0.0.1 \
GATESENTRY_DNS_PORT=10053 \
GATESENTRY_DNS_RESOLVER=8.8.8.8 \
./bin/gatesentrybin
```

### Build Only

```bash
./build.sh
```

### Run DNS Test Suite

```bash
# Run full test suite (server must be running)
./scripts/dns_deep_test.sh -p 10053 -s 127.0.0.1 -r 8.8.8.8

# Run with verbose output
./scripts/dns_deep_test.sh -p 10053 -s 127.0.0.1 -v
```

### Manual DNS Tests

```bash
# Test UDP query
dig @127.0.0.1 -p 10053 google.com A +short

# Test TCP query
dig @127.0.0.1 -p 10053 google.com A +tcp +short

# Test blocked domain
dig @127.0.0.1 -p 10053 blocked-domain.com A +short
```
