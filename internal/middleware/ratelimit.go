package middleware

import (
	"net/http"
	"sync"
	"time"
)

type visitor struct {
	tokens    float64
	lastCheck time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64
	burst    float64
	cleanup  *time.Ticker
}

func NewRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     float64(requestsPerSecond),
		burst:    float64(burst),
		cleanup:  time.NewTicker(5 * time.Minute),
	}

	go func() {
		for range rl.cleanup.C {
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, v := range rl.visitors {
				if v.lastCheck.Before(cutoff) {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()

	return rl
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists {
		rl.visitors[ip] = &visitor{tokens: rl.burst - 1, lastCheck: now}
		return true
	}

	elapsed := now.Sub(v.lastCheck).Seconds()
	v.tokens += elapsed * rl.rate
	if v.tokens > rl.burst {
		v.tokens = rl.burst
	}
	v.lastCheck = now

	if v.tokens < 1 {
		return false
	}

	v.tokens--
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "5")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded","retry_after":5}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
