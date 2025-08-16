package extension

import (
	"github.com/litesql/httpcache/config"
	"github.com/walterwanderley/sqlite"
)

func registerFunc(api *sqlite.ExtensionApi) (sqlite.ErrorCode, error) {
	if err := api.CreateModule(config.DefaultVirtualTableName, &CacheModule{}, sqlite.ReadOnly(false)); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_info", &Info{}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_age", &Age{}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_lifetime", &FreshnessLifetime{}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_lifetime_shared", &FreshnessLifetime{
		shared: true,
	}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_expired", &Expired{}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	if err := api.CreateFunction("cache_expired_ttl", &Expired{
		fallbackTTL: true,
	}); err != nil {
		return sqlite.SQLITE_ERROR, err
	}
	return sqlite.SQLITE_OK, nil
}
