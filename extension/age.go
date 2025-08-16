package extension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/walterwanderley/sqlite"

	cachehttp "github.com/litesql/httpcache/http"
)

type Age struct {
}

func (m *Age) Args() int {
	return 3
}

func (m *Age) Deterministic() bool {
	return true
}

func (m *Age) Apply(ctx *sqlite.Context, values ...sqlite.Value) {
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

	age := cachehttp.Age(header, requestTime, responseTime)
	if age == nil {
		ctx.ResultNull()
		return
	}

	ctx.ResultInt(*age)
}
