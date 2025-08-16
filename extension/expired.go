package extension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/walterwanderley/sqlite"

	cachehttp "github.com/litesql/httpcache/http"
)

type Expired struct {
	fallbackTTL bool
}

func (m *Expired) Args() int {
	if m.fallbackTTL {
		return 5
	}
	return 4
}
func (m *Expired) Deterministic() bool { return true }
func (m *Expired) Apply(ctx *sqlite.Context, values ...sqlite.Value) {
	var header http.Header
	if err := json.Unmarshal([]byte(values[0].Text()), &header); err != nil {
		ctx.ResultError(fmt.Errorf("invalid header: %w", err))
		return
	}

	requestTime, err := time.Parse(time.RFC3339Nano, values[1].Text())
	if err != nil {
		ctx.ResultError(fmt.Errorf("invalid request time: %w", err))
		return
	}
	responseTime, err := time.Parse(time.RFC3339Nano, values[2].Text())
	if err != nil {
		ctx.ResultError(fmt.Errorf("invalid response time: %w", err))
		return
	}

	var shared bool
	if v, err := strconv.ParseBool(values[3].Text()); err != nil {
		ctx.ResultError(fmt.Errorf("invalid 'shared' param: %v", err))
		return
	} else {
		shared = v
	}

	var ttl int
	if m.fallbackTTL {
		if v, err := strconv.Atoi(values[4].Text()); err != nil {
			ctx.ResultError(fmt.Errorf("invalid 'shared' param: %v", err))
			return
		} else {
			ttl = v
		}
	}

	cacheControl := cachehttp.ParseCacheControl(header, &requestTime, &responseTime, shared, ttl)
	if cacheControl.Expired() {
		ctx.ResultInt(1)
	} else {
		ctx.ResultInt(0)
	}
}
