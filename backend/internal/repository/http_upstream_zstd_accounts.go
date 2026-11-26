package repository

import (
	"context"
	"database/sql"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const upstreamZstdAccountsRefreshInterval = 15 * time.Second

// 只加载显式开了压缩的 API Key 账号。migration 246 已按「写放大和维护成本大于收益」
// 删掉了通用 extra GIN 索引，这条 containment 查询因此是顺序扫描；本表规模很小（单站
// 几十上百行）且最多 15 秒跑一次，代价可忽略。将来账号数上万再补一条只覆盖该键的窄索引。
const upstreamZstdAccountsQuery = `SELECT id FROM accounts WHERE deleted_at IS NULL AND type = 'apikey' AND extra @> '{"` + service.AccountExtraUpstreamRequestZstd + `":true}'::jsonb`

// The shared transport receives account IDs, not account objects. Keep the indexed
// lookup off the request path and publish a snapshot only after a complete read.
type upstreamZstdAccounts struct {
	db       *sql.DB
	ids      atomic.Pointer[map[int64]struct{}]
	loadedAt atomic.Int64
	loading  atomic.Bool
}

func ProvideHTTPUpstream(cfg *config.Config, db *sql.DB) service.HTTPUpstream {
	s := newHTTPUpstream(cfg)
	if db != nil {
		s.zstdAccounts = &upstreamZstdAccounts{db: db}
		s.zstdAccounts.refresh(context.Background())
	}
	return s
}

func (r *upstreamZstdAccounts) enabled(accountID int64) bool {
	if r == nil || accountID <= 0 {
		return false
	}
	if time.Since(time.Unix(0, r.loadedAt.Load())) >= upstreamZstdAccountsRefreshInterval && r.loading.CompareAndSwap(false, true) {
		go func() {
			defer r.loading.Store(false)
			r.refresh(context.Background())
		}()
	}
	ids := r.ids.Load()
	if ids == nil {
		return false
	}
	_, enabled := (*ids)[accountID]
	return enabled
}

func (r *upstreamZstdAccounts) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defer func() { r.loadedAt.Store(time.Now().UnixNano()) }()
	ids, err := r.loadIDs(ctx)
	if err != nil {
		slog.Warn("failed to refresh upstream request compression settings", "error", err)
		return
	}
	r.ids.Store(&ids)
}

func (r *upstreamZstdAccounts) loadIDs(ctx context.Context) (map[int64]struct{}, error) {
	rows, err := r.db.QueryContext(ctx, upstreamZstdAccountsQuery)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}
