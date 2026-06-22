package http

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate-limit budget for /api/v1/auth/* (spec: 10 requests/min per IP).
const (
	rateLimitRequests = 10
	rateLimitWindow   = time.Minute
	visitorIdleTTL    = 3 * time.Minute
	cleanupInterval   = time.Minute
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ipRateLimiter is an in-memory per-IP token-bucket limiter. State is per process
// instance (acceptable for the single always-on instance; design D7). Stale
// entries are swept lazily to bound memory without a background goroutine.
type ipRateLimiter struct {
	mu          sync.Mutex
	visitors    map[string]*visitor
	limit       rate.Limit
	burst       int
	lastCleanup time.Time
}

func newIPRateLimiter() *ipRateLimiter {
	return &ipRateLimiter{
		visitors:    make(map[string]*visitor),
		limit:       rate.Every(rateLimitWindow / rateLimitRequests),
		burst:       rateLimitRequests,
		lastCleanup: time.Now(),
	}
}

func (l *ipRateLimiter) allow(address string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	currentTime := time.Now()
	if currentTime.Sub(l.lastCleanup) > cleanupInterval {
		for key, seen := range l.visitors {
			if currentTime.Sub(seen.lastSeen) > visitorIdleTTL {
				delete(l.visitors, key)
			}
		}
		l.lastCleanup = currentTime
	}

	seen, ok := l.visitors[address]
	if !ok {
		seen = &visitor{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[address] = seen
	}
	seen.lastSeen = currentTime

	return seen.limiter.Allow()
}

// rateLimit rejects requests over the per-IP budget with 429 RATE_LIMITED.
func rateLimit(limiter *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(clientAddress(r)) {
				writeErrorCode(w, http.StatusTooManyRequests, codeRateLimited, userMessage(codeRateLimited))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientAddress is the request's client IP. Behind Render's single proxy we
// trust the leftmost X-Forwarded-For entry (spoofable — design D7 trade-off,
// documented; the first thing to harden off free tier), falling back to the
// transport peer for direct/local requests.
func clientAddress(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if comma := strings.IndexByte(forwarded, ','); comma >= 0 {
			return strings.TrimSpace(forwarded[:comma])
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
