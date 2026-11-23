package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FZambia/sentinel"
	"github.com/gomodule/redigo/redis"
	"github.com/ys-ll/uniterm/backend/log"
)

// errRedisNil mirrors go-redis' redis.Nil error message so frontend error
// handling keeps seeing the same "redis: nil" string for missing keys.
var errRedisNil = errors.New("redis: nil")

// ScanResult holds a page of SCAN results with cursor for pagination.
type ScanResult struct {
	Keys      []RedisKeyInfo `json:"keys"`
	Cursor    uint64         `json:"cursor"`
	ScanCount int            `json:"scanCount"`
}

// RedisKeyInfo holds metadata for a single Redis key.
type RedisKeyInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`
}

// FieldEntry represents a hash field-value pair.
type FieldEntry struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// ScoredMember represents a sorted-set member with its score.
type ScoredMember struct {
	Score  float64 `json:"score"`
	Member  string  `json:"member"`
}

// RedisSession implements the Session interface for Redis connections using
// redigo (a thin RESP client) instead of go-redis to keep the binary small.
type RedisSession struct {
	baseSession
	pool   *redis.Pool
	dbIdx  int
	closed bool
}

// NewRedisSession creates a new RedisSession with the given ID.
func NewRedisSession(id string) *RedisSession {
	return &RedisSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "redis",
			status:      StatusDisconnected,
		},
		dbIdx: 0,
	}
}

// redigoDialOptions builds the dial options shared by every connection mode.
func redigoDialOptions(user, password string, upstream *SocksProxy) []redis.DialOption {
	opts := []redis.DialOption{
		redis.DialConnectTimeout(10 * time.Second),
		redis.DialReadTimeout(30 * time.Second),
		redis.DialWriteTimeout(30 * time.Second),
	}
	if user != "" {
		opts = append(opts, redis.DialUsername(user))
	}
	if password != "" {
		opts = append(opts, redis.DialPassword(password))
	}
	if upstream != nil {
		proxyDial := ProxyDialFunc(upstream)
		opts = append(opts, redis.DialNetDial(func(network, addr string) (net.Conn, error) {
			return proxyDial(context.Background(), network, addr)
		}))
	}
	return opts
}

// dialWithDB dials a Redis connection and re-selects the session's database
// index, so pooled connections land on the same db as the live ones.
func dialWithDB(s *RedisSession, network, addr string, upstream *SocksProxy, user, password string) (redis.Conn, error) {
	opts := redigoDialOptions(user, password, upstream)
	conn, err := redis.Dial(network, addr, opts...)
	if err != nil {
		return nil, err
	}
	if idx := s.DBIndex(); idx > 0 {
		if _, err := conn.Do("SELECT", idx); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// Connect establishes a connection to the Redis server.
func (s *RedisSession) Connect(config ConnectionConfig) error {
	log.Writef("[RedisSession.Connect] id=%s, host=%s, port=%d", s.id, config.Host, config.Port)
	s.setStatus(StatusConnecting)

	if config.Name != "" {
		s.title = config.Name
	} else if config.RedisMode == "sentinel" {
		s.title = fmt.Sprintf("redis:%s", config.RedisMasterName)
	} else {
		s.title = fmt.Sprintf("redis:%s:%d", config.Host, config.Port)
	}

	var pool *redis.Pool
	if config.RedisMode == "sentinel" {
		addrs := make([]string, 0)
		for _, a := range strings.Split(config.RedisSentinels, ",") {
			if a = strings.TrimSpace(a); a != "" {
				addrs = append(addrs, a)
			}
		}
		if len(addrs) == 0 {
			s.setStatus(StatusError)
			return fmt.Errorf("redis sentinel: no sentinel addresses")
		}
		if config.RedisMasterName == "" {
			s.setStatus(StatusError)
			return fmt.Errorf("redis sentinel: master name required")
		}
		pool = s.sentinelPool(config, addrs)
	} else {
		pool = s.standalonePool(config)
	}

	conn, err := pool.GetContext(context.Background())
	if err != nil {
		log.Writef("[RedisSession.Connect] dial failed: %v", err)
		pool.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("redis dial: %w", err)
	}
	if _, err := redis.DoWithTimeout(conn, 10*time.Second, "PING"); err != nil {
		log.Writef("[RedisSession.Connect] PING failed: %v", err)
		conn.Close()
		pool.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("redis ping: %w", err)
	}
	conn.Close()

	s.mu.Lock()
	s.pool = pool
	s.mu.Unlock()

	log.Writef("[RedisSession.Connect] connected successfully")
	s.setStatus(StatusConnected)
	return nil
}

// standalonePool returns a pool dialing host:port directly (proxy-aware).
func (s *RedisSession) standalonePool(config ConnectionConfig) *redis.Pool {
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return dialWithDB(s, "tcp", addr, config.Proxy, config.User, config.Password)
		},
		TestOnBorrow: testOnBorrow,
	}
}

// sentinelPool returns a pool that resolves the current master through the
// sentinels on every dial, so failover is picked up on reconnect.
func (s *RedisSession) sentinelPool(config ConnectionConfig, addrs []string) *redis.Pool {
	sntnl := &sentinel.Sentinel{
		Addrs:      addrs,
		MasterName: config.RedisMasterName,
		Dial: func(addr string) (redis.Conn, error) {
			opts := []redis.DialOption{
				redis.DialConnectTimeout(10 * time.Second),
				redis.DialReadTimeout(5 * time.Second),
				redis.DialWriteTimeout(5 * time.Second),
			}
			if config.SentinelUser != "" {
				opts = append(opts, redis.DialUsername(config.SentinelUser))
			}
			if config.SentinelPassword != "" {
				opts = append(opts, redis.DialPassword(config.SentinelPassword))
			}
			if config.Proxy != nil {
				proxyDial := ProxyDialFunc(config.Proxy)
				opts = append(opts, redis.DialNetDial(func(network, a string) (net.Conn, error) {
					return proxyDial(context.Background(), network, a)
				}))
			}
			return redis.Dial("tcp", addr, opts...)
		},
	}
	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			masterAddr, err := sntnl.MasterAddr()
			if err != nil {
				return nil, fmt.Errorf("redis sentinel: resolve master: %w", err)
			}
			return dialWithDB(s, "tcp", masterAddr, config.Proxy, config.User, config.Password)
		},
		TestOnBorrow: testOnBorrow,
	}
}

// testOnBorrow pings idle connections before reuse.
func testOnBorrow(c redis.Conn, t time.Time) error {
	if time.Since(t) < time.Minute {
		return nil
	}
	_, err := redis.DoWithTimeout(c, 5*time.Second, "PING")
	return err
}

// Disconnect closes the Redis connection pool.
func (s *RedisSession) Disconnect() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.pool != nil {
		s.pool.Close()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

// IsConnected returns true if the session is connected and the pool is valid.
func (s *RedisSession) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status == StatusConnected && s.pool != nil
}

// Write is a no-op for Redis sessions (no interactive terminal I/O).
func (s *RedisSession) Write(data []byte) error { return nil }

// Resize is a no-op for Redis sessions (no terminal dimensions).
func (s *RedisSession) Resize(cols, rows int) error { return nil }

// Client returns the underlying connection pool.
func (s *RedisSession) Client() *redis.Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool
}

// do runs a single command with the given timeout on a pooled connection.
func (s *RedisSession) do(timeout time.Duration, cmd string, args ...interface{}) (interface{}, error) {
	pool := s.Client()
	if pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	conn := pool.Get()
	defer conn.Close()
	return redis.DoWithTimeout(conn, timeout, cmd, args...)
}

// pipeline runs Send/Flush/Receive batches on one pooled connection. The
// callback sends commands via send, then must call flush before receiving
// replies — redigo buffers Send calls, so Receive without Flush blocks until
// the read timeout.
func (s *RedisSession) pipeline(timeout time.Duration, fn func(send func(cmd string, args ...interface{}) error, flush func() error, receive func() (interface{}, error)) error) error {
	pool := s.Client()
	if pool == nil {
		return fmt.Errorf("not connected")
	}
	conn := pool.Get()
	defer conn.Close()

	send := func(cmd string, args ...interface{}) error {
		return conn.Send(cmd, args...)
	}
	flush := func() error {
		return conn.Flush()
	}
	receive := func() (interface{}, error) {
		return redis.ReceiveWithTimeout(conn, timeout)
	}
	return fn(send, flush, receive)
}

// DBIndex returns the current database index.
func (s *RedisSession) DBIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dbIdx
}

// Ping checks connectivity to the Redis server.
func (s *RedisSession) Ping() error {
	_, err := s.do(5*time.Second, "PING")
	return err
}

// SwitchDB switches to the specified Redis database index.
func (s *RedisSession) SwitchDB(idx int) error {
	if idx < 0 {
		return fmt.Errorf("invalid db index: %d (must be >= 0)", idx)
	}
	var switchErr error
	err := s.pipeline(5*time.Second, func(send func(cmd string, args ...interface{}) error, flush func() error, receive func() (interface{}, error)) error {
		if err := send("SELECT", idx); err != nil {
			return err
		}
		if err := send("PING"); err != nil {
			return err
		}
		if err := flush(); err != nil {
			return err
		}
		if _, err := receive(); err != nil {
			switchErr = err
			return nil
		}
		if _, err := receive(); err != nil {
			switchErr = err
		}
		return nil
	})
	if err != nil || switchErr != nil {
		log.Writef("[RedisSession.SwitchDB] failed to switch to db %d: %v", idx, firstErr(err, switchErr))
		return fmt.Errorf("switch db %d: %w", idx, firstErr(err, switchErr))
	}

	s.mu.Lock()
	s.dbIdx = idx
	s.mu.Unlock()
	return nil
}

// DBCount returns the total number of databases configured on the server.
// It queries CONFIG GET databases; if the server disallows CONFIG, it falls
// back to the highest db index reported by INFO keyspace plus one, and
// finally to the Redis default of 16.
func (s *RedisSession) DBCount() (int, error) {
	reply, err := s.do(5*time.Second, "CONFIG", "GET", "databases")
	if err != nil {
		log.Writef("[RedisSession.DBCount] CONFIG GET databases failed: %v", err)
	} else if pair, ok := reply.([]interface{}); ok && len(pair) == 2 {
		if v, ok := pair[1].([]byte); ok {
			if n, parseErr := strconv.Atoi(string(v)); parseErr == nil && n > 0 {
				return n, nil
			}
		}
	}

	// Fallback: INFO keyspace only lists non-empty dbs, so this may
	// under-report the total on an empty server.
	if ks, err := s.KeyspaceInfo(); err == nil {
		max := 0
		for idx := range ks {
			if idx+1 > max {
				max = idx + 1
			}
		}
		if max > 0 {
			return max, nil
		}
	}
	return 16, nil
}

// parseScanReply splits a SCAN reply into (keys, nextCursor).
func parseScanReply(reply interface{}) ([]string, uint64, error) {
	pair, ok := reply.([]interface{})
	if !ok || len(pair) != 2 {
		return nil, 0, fmt.Errorf("scan: unexpected reply shape")
	}
	rawCursor, ok := pair[0].([]byte)
	if !ok {
		return nil, 0, fmt.Errorf("scan: unexpected cursor type")
	}
	nextCursor, err := strconv.ParseUint(string(rawCursor), 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf("scan: bad cursor %q: %w", rawCursor, err)
	}
	keys := make([]string, 0, len(pair[1].([]interface{})))
	if items, ok := pair[1].([]interface{}); ok {
		for _, item := range items {
			if kb, ok := item.([]byte); ok {
				keys = append(keys, string(kb))
			}
		}
	}
	return keys, nextCursor, nil
}

// ScanKeys scans keys matching pattern with cursor-based pagination.
// Returns key metadata (name, type, TTL) via pipeline batching.
func (s *RedisSession) ScanKeys(pattern string, cursor uint64, count int64) (*ScanResult, error) {
	if count <= 0 {
		count = 100
	}
	reply, err := s.do(10*time.Second, "SCAN", strconv.FormatUint(cursor, 10), "MATCH", pattern, "COUNT", count)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	keys, nextCursor, err := parseScanReply(reply)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		Keys:      make([]RedisKeyInfo, 0, len(keys)),
		Cursor:    nextCursor,
		ScanCount: len(keys),
	}
	if len(keys) == 0 {
		return result, nil
	}

	// Batch TYPE + TTL via pipeline
	type typesAndTTLs struct {
		types []string
		ttls  []int64
	}
	batch := &typesAndTTLs{
		types: make([]string, len(keys)),
		ttls:  make([]int64, len(keys)),
	}
	err = s.pipeline(10*time.Second, func(send func(cmd string, args ...interface{}) error, flush func() error, receive func() (interface{}, error)) error {
		for _, key := range keys {
			if err := send("TYPE", key); err != nil {
				return err
			}
			if err := send("TTL", key); err != nil {
				return err
			}
		}
		if err := flush(); err != nil {
			return err
		}
		for i := range keys {
			typeReply, err := receive()
			if err != nil {
				return err
			}
			if t, ok := typeReply.(string); ok {
				batch.types[i] = string(t)
			}
			ttlReply, err := receive()
			if err != nil {
				return err
			}
			if ttl, ok := ttlReply.(int64); ok {
				batch.ttls[i] = ttl
			}
		}
		return nil
	})
	if err != nil && err != redis.ErrNil {
		return nil, fmt.Errorf("scan pipeline: %w", err)
	}

	for i, key := range keys {
		t := batch.types[i]
		if t == "none" {
			continue
		}
		ttl := int64(-1)
		if batch.ttls[i] > 0 {
			ttl = batch.ttls[i]
		} else if batch.ttls[i] == -2 {
			ttl = -2
		}
		result.Keys = append(result.Keys, RedisKeyInfo{
			Name: key,
			Type: t,
			TTL:  ttl,
		})
	}
	return result, nil
}

// GetKeyInfo returns metadata (type, TTL) for a single key.
func (s *RedisSession) GetKeyInfo(key string) (*RedisKeyInfo, error) {
	info := &RedisKeyInfo{Name: key}
	err := s.pipeline(5*time.Second, func(send func(cmd string, args ...interface{}) error, flush func() error, receive func() (interface{}, error)) error {
		if err := send("TYPE", key); err != nil {
			return err
		}
		if err := send("TTL", key); err != nil {
			return err
		}
		if err := flush(); err != nil {
			return err
		}
		typeReply, err := receive()
		if err != nil {
			return err
		}
		if t, ok := typeReply.(string); ok {
			info.Type = string(t)
		}
		ttlReply, err := receive()
		if err != nil {
			return err
		}
		if ttl, ok := ttlReply.(int64); ok {
			info.TTL = ttl
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get key info: %w", err)
	}
	if info.Type == "none" {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	ttl := int64(-1)
	if info.TTL > 0 {
		ttl = info.TTL
	} else if info.TTL == -2 {
		ttl = -2
	}
	info.TTL = ttl
	return info, nil
}

// DBSize returns the number of keys in the current database.
func (s *RedisSession) DBSize() (int64, error) {
	reply, err := s.do(5*time.Second, "DBSIZE")
	if err != nil {
		return 0, err
	}
	if n, ok := reply.(int64); ok {
		return n, nil
	}
	return 0, fmt.Errorf("dbsize: unexpected reply type")
}

// KeyspaceInfo returns key counts for all databases via INFO keyspace.
// Returns a map of db index -> key count. Empty dbs are omitted by Redis.
func (s *RedisSession) KeyspaceInfo() (map[int]int64, error) {
	reply, err := s.do(5*time.Second, "INFO", "keyspace")
	if err != nil {
		return nil, err
	}
	info, ok := reply.([]byte)
	if !ok {
		return nil, fmt.Errorf("info: unexpected reply type")
	}
	return parseKeyspaceInfo(string(info)), nil
}

// parseKeyspaceInfo extracts db index -> key counts from a INFO keyspace
// payload ("db0:keys=2,expires=0,avg_ttl=0" lines).
func parseKeyspaceInfo(info string) map[int]int64 {
	result := make(map[int]int64)
	for _, line := range strings.Split(info, "\r\n") {
		if strings.HasPrefix(line, "db") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			var dbIdx int
			if _, scanErr := fmt.Sscanf(parts[0], "db%d", &dbIdx); scanErr != nil {
				continue
			}
			for _, pair := range strings.Split(parts[1], ",") {
				if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 && kv[0] == "keys" {
					if n, parseErr := strconv.ParseInt(kv[1], 10, 64); parseErr == nil {
						result[dbIdx] = n
					}
				}
			}
		}
	}
	return result
}

// DeleteKey deletes a key from Redis.
func (s *RedisSession) DeleteKey(key string) error {
	_, err := s.do(5*time.Second, "DEL", key)
	return err
}

// KeyExists returns true if the key exists in Redis.
func (s *RedisSession) KeyExists(key string) (bool, error) {
	reply, err := s.do(5*time.Second, "EXISTS", key)
	if err != nil {
		return false, err
	}
	n, ok := reply.(int64)
	return ok && n > 0, nil
}

// GetKeyTTL returns the TTL for a key in seconds.
func (s *RedisSession) GetKeyTTL(key string) (int64, error) {
	reply, err := s.do(5*time.Second, "TTL", key)
	if err != nil {
		return -2, err
	}
	ttl, ok := reply.(int64)
	if !ok {
		return -2, fmt.Errorf("ttl: unexpected reply type")
	}
	return ttl, nil
}

// SetKeyTTL sets the TTL for a key in seconds.
func (s *RedisSession) SetKeyTTL(key string, seconds int64) error {
	if seconds < 0 {
		_, err := s.do(5*time.Second, "PERSIST", key)
		return err
	}
	_, err := s.do(5*time.Second, "EXPIRE", key, seconds)
	return err
}

// --- String operations ---

// GetString returns the string value of a key.
func (s *RedisSession) GetString(key string) (string, error) {
	reply, err := s.do(5*time.Second, "GET", key)
	if err != nil {
		return "", err
	}
	if v, ok := reply.([]byte); ok {
		return string(v), nil
	}
	return "", errRedisNil
}

// SetString sets a string value for a key.
func (s *RedisSession) SetString(key string, value string) error {
	_, err := s.do(5*time.Second, "SET", key, value)
	return err
}

// --- Hash operations ---

// GetHashAll returns all field-value pairs of a hash.
func (s *RedisSession) GetHashAll(key string) ([]FieldEntry, error) {
	reply, err := s.do(5*time.Second, "HGETALL", key)
	if err != nil {
		return nil, err
	}
	items, ok := reply.([]interface{})
	if !ok {
		return nil, fmt.Errorf("hgetall: unexpected reply type")
	}
	entries := make([]FieldEntry, 0, len(items)/2)
	for i := 0; i+1 < len(items); i += 2 {
		field, _ := items[i].([]byte)
		value, _ := items[i+1].([]byte)
		entries = append(entries, FieldEntry{Field: string(field), Value: string(value)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Field < entries[j].Field })
	return entries, nil
}

// HashSet sets a field in a hash.
func (s *RedisSession) HashSet(key string, field string, value string) error {
	_, err := s.do(5*time.Second, "HSET", key, field, value)
	return err
}

// HashDel deletes one or more fields from a hash.
func (s *RedisSession) HashDel(key string, fields []string) (int64, error) {
	args := make([]interface{}, 0, len(fields)+1)
	args = append(args, key)
	for _, f := range fields {
		args = append(args, f)
	}
	reply, err := s.do(5*time.Second, "HDEL", args...)
	if err != nil {
		return 0, err
	}
	if n, ok := reply.(int64); ok {
		return n, nil
	}
	return 0, fmt.Errorf("hdel: unexpected reply type")
}

// --- List operations ---

// GetListRange returns elements from a list within the specified range.
func (s *RedisSession) GetListRange(key string, start int64, stop int64) ([]string, error) {
	reply, err := s.do(5*time.Second, "LRANGE", key, start, stop)
	if err != nil {
		return nil, err
	}
	items, ok := reply.([]interface{})
	if !ok {
		return nil, fmt.Errorf("lrange: unexpected reply type")
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if b, ok := item.([]byte); ok {
			values = append(values, string(b))
		}
	}
	return values, nil
}

// ListPush pushes values onto a list from the specified direction ("left" or "right").
func (s *RedisSession) ListPush(key string, direction string, values []string) (int64, error) {
	args := make([]interface{}, 0, len(values)+1)
	args = append(args, key)
	for _, v := range values {
		args = append(args, v)
	}
	cmd := "RPUSH"
	if direction == "left" {
		cmd = "LPUSH"
	}
	reply, err := s.do(5*time.Second, cmd, args...)
	if err != nil {
		return 0, err
	}
	if n, ok := reply.(int64); ok {
		return n, nil
	}
	return 0, fmt.Errorf("%s: unexpected reply type", strings.ToLower(cmd))
}

// ListPop pops a value from a list from the specified direction ("left" or "right").
func (s *RedisSession) ListPop(key string, direction string) (string, error) {
	cmd := "RPOP"
	if direction == "left" {
		cmd = "LPOP"
	}
	reply, err := s.do(5*time.Second, cmd, key)
	if err != nil {
		return "", err
	}
	if v, ok := reply.([]byte); ok {
		return string(v), nil
	}
	return "", errRedisNil
}

// ListSet sets the value at an index in a list.
func (s *RedisSession) ListSet(key string, index int64, value string) error {
	_, err := s.do(5*time.Second, "LSET", key, index, value)
	return err
}

// ListRemove removes elements from a list matching the value.
func (s *RedisSession) ListRemove(key string, value string, count int64) (int64, error) {
	reply, err := s.do(5*time.Second, "LREM", key, count, value)
	if err != nil {
		return 0, err
	}
	if n, ok := reply.(int64); ok {
		return n, nil
	}
	return 0, fmt.Errorf("lrem: unexpected reply type")
}

// --- Set operations ---

// GetSetAll returns all members of a set.
func (s *RedisSession) GetSetAll(key string) ([]string, error) {
	reply, err := s.do(5*time.Second, "SMEMBERS", key)
	if err != nil {
		return nil, err
	}
	items, ok := reply.([]interface{})
	if !ok {
		return nil, fmt.Errorf("smembers: unexpected reply type")
	}
	members := make([]string, 0, len(items))
	for _, item := range items {
		if b, ok := item.([]byte); ok {
			members = append(members, string(b))
		}
	}
	return members, nil
}

// SetAdd adds members to a set.
func (s *RedisSession) SetAdd(key string, members []string) (int64, error) {
	reply, err := s.do(5*time.Second, "SADD", strSliceCmd(key, members)...)
	if err != nil {
		return 0, err
	}
	return replyAsInt64(reply, "sadd")
}

// SetRemove removes members from a set.
func (s *RedisSession) SetRemove(key string, members []string) (int64, error) {
	reply, err := s.do(5*time.Second, "SREM", strSliceCmd(key, members)...)
	if err != nil {
		return 0, err
	}
	return replyAsInt64(reply, "srem")
}

// strSliceCmd builds args for "CMD key members..." calls.
func strSliceCmd(key string, values []string) []interface{} {
	args := make([]interface{}, 0, len(values)+1)
	args = append(args, key)
	for _, v := range values {
		args = append(args, v)
	}
	return args
}

// replyAsInt64 converts an integer reply, reporting unexpected shapes.
func replyAsInt64(reply interface{}, op string) (int64, error) {
	if n, ok := reply.(int64); ok {
		return n, nil
	}
	return 0, fmt.Errorf("%s: unexpected reply type", op)
}

// --- Sorted Set operations ---

// GetSortedSetRange returns members within a score range, with scores.
func (s *RedisSession) GetSortedSetRange(key string, min string, max string) ([]ScoredMember, error) {
	reply, err := s.do(5*time.Second, "ZRANGEBYSCORE", key, min, max, "WITHSCORES")
	if err != nil {
		return nil, err
	}
	items, ok := reply.([]interface{})
	if !ok {
		return nil, fmt.Errorf("zrangebyscore: unexpected reply type")
	}
	members := make([]ScoredMember, 0, len(items)/2)
	for i := 0; i+1 < len(items); i += 2 {
		member, _ := items[i].([]byte)
		scoreBytes, _ := items[i+1].([]byte)
		score, parseErr := strconv.ParseFloat(string(scoreBytes), 64)
		if parseErr != nil {
			return nil, fmt.Errorf("zrangebyscore: bad score %q: %w", scoreBytes, parseErr)
		}
		members = append(members, ScoredMember{Member: string(member), Score: score})
	}
	return members, nil
}

// ZSetAdd adds members with scores to a sorted set.
func (s *RedisSession) ZSetAdd(key string, members []ScoredMember) (int64, error) {
	args := make([]interface{}, 0, len(members)*2+1)
	args = append(args, key)
	for _, m := range members {
		args = append(args, m.Score, m.Member)
	}
	reply, err := s.do(5*time.Second, "ZADD", args...)
	if err != nil {
		return 0, err
	}
	return replyAsInt64(reply, "zadd")
}

// ZSetRemove removes members from a sorted set.
func (s *RedisSession) ZSetRemove(key string, members []string) (int64, error) {
	reply, err := s.do(5*time.Second, "ZREM", strSliceCmd(key, members)...)
	if err != nil {
		return 0, err
	}
	return replyAsInt64(reply, "zrem")
}
