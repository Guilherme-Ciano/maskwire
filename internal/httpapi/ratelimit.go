package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    func() time.Time
}

func NewLimiter(rps float64, burst int) *Limiter {
	if rps <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = max(1, int(math.Ceil(rps)))
	}
	return &Limiter{rate: rps, burst: float64(burst), tokens: float64(burst), now: time.Now}
}

func (l *Limiter) Allow() (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.last.IsZero() {
		l.tokens = min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.rate)
	}
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return true, 0
	}
	return false, time.Duration((1 - l.tokens) / l.rate * float64(time.Second))
}

func (s *Server) admit(w http.ResponseWriter, tenant *Tenant, rt *Route) bool {
	ok, retry := tenant.Limit.Allow()
	if ok {
		return true
	}
	secs := max(1, int(math.Ceil(retry.Seconds())))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "rate_limited", ErrRateLimited.Error())
	s.metrics.Requests.WithLabelValues(tenant.Mask.ID, rt.Name, strconv.Itoa(http.StatusTooManyRequests)).Inc()
	return false
}
