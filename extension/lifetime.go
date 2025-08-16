package extension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/walterwanderley/sqlite"

	cachehttp "github.com/litesql/httpcache/http"
)

type FreshnessLifetime struct {
	shared bool
}

func (m *FreshnessLifetime) Args() int {
	return 2
}

func (m *FreshnessLifetime) Deterministic() bool {
	return true
}

func (m *FreshnessLifetime) Apply(ctx *sqlite.Context, values ...sqlite.Value) {
	var header http.Header
	if err := json.Unmarshal([]byte(values[0].Text()), &header); err != nil {
		ctx.ResultNull()
		return
	}

	var responseTime *time.Time
	if !values[1].IsNil() {
		if v, err := time.Parse(time.RFC3339Nano, values[1].Text()); err != nil {
			ctx.ResultError(fmt.Errorf("invalid 'response_time' param: %v", err))
			return
		} else {
			responseTime = &v
		}
	}

	cacheControl := cachehttp.ParseCacheControl(header, nil, responseTime, m.shared, 0)
	ctx.ResultInt(cacheControl.FreshnessLifetime())
}
