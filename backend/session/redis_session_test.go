package session

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// newTestRedis starts a miniredis (a pure-Go in-memory Redis) with the given
// password, so the suite runs with plain `go test` and no external services.
func newTestRedis(t *testing.T, password string) *miniredis.Miniredis {
	t.Helper()
	mr := miniredis.RunT(t)
	if password != "" {
		mr.RequireAuth(password)
	}
	return mr
}

// newTestSession connects a RedisSession to the given miniredis (standalone).
func newTestSession(t *testing.T, mr *miniredis.Miniredis, password string) *RedisSession {
	t.Helper()
	s := NewRedisSession("test-" + t.Name())
	if err := s.Connect(mrConnectConfig(mr, password)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Disconnect() })
	return s
}

func mrConnectConfig(mr *miniredis.Miniredis, password string) ConnectionConfig {
	host, portStr, _ := net.SplitHostPort(mr.Addr())
	port, _ := strconv.Atoi(portStr)
	return ConnectionConfig{Host: host, Port: port, Password: password}
}

func TestRedisStringOps(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	if err := s.SetString("foo", "bar"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, err := s.GetString("foo")
	if err != nil || v != "bar" {
		t.Fatalf("get: got %q, %v", v, err)
	}

	// Missing key maps to the go-redis compatible nil error message.
	if _, err := s.GetString("missing"); err == nil || err.Error() != "redis: nil" {
		t.Fatalf("missing key: want \"redis: nil\", got %v", err)
	}

	if ok, err := s.KeyExists("foo"); err != nil || !ok {
		t.Fatalf("exists: got %v, %v", ok, err)
	}
	if ok, _ := s.KeyExists("missing"); ok {
		t.Fatal("missing key reported as existing")
	}

	if err := s.DeleteKey("foo"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if ok, _ := s.KeyExists("foo"); ok {
		t.Fatal("key survived delete")
	}
}

func TestRedisAuthFailure(t *testing.T) {
	mr := newTestRedis(t, "secret")
	s := NewRedisSession("test-auth")
	cfg := mrConnectConfig(mr, "wrong")
	if err := s.Connect(cfg); err == nil {
		s.Disconnect()
		t.Fatal("connect with wrong password should fail")
	}
	if s.IsConnected() {
		t.Fatal("session marked connected after failed auth")
	}
}

func TestRedisHashOps(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	if err := s.HashSet("h", "zeta", "1"); err != nil {
		t.Fatalf("hset: %v", err)
	}
	if err := s.HashSet("h", "alpha", "2"); err != nil {
		t.Fatalf("hset: %v", err)
	}
	entries, err := s.GetHashAll("h")
	if err != nil {
		t.Fatalf("hgetall: %v", err)
	}
	// Fields must come back sorted regardless of insertion order.
	if len(entries) != 2 || entries[0].Field != "alpha" || entries[1].Field != "zeta" {
		t.Fatalf("hgetall: got %+v", entries)
	}

	n, err := s.HashDel("h", []string{"alpha", "ghost"})
	if err != nil || n != 1 {
		t.Fatalf("hdel: got %d, %v", n, err)
	}
}

func TestRedisListOps(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	if n, err := s.ListPush("l", "right", []string{"a", "b", "c"}); err != nil || n != 3 {
		t.Fatalf("rpush: got %d, %v", n, err)
	}
	if n, err := s.ListPush("l", "left", []string{"head"}); err != nil || n != 4 {
		t.Fatalf("lpush: got %d, %v", n, err)
	}
	items, err := s.GetListRange("l", 0, -1)
	if err != nil {
		t.Fatalf("lrange: %v", err)
	}
	want := []string{"head", "a", "b", "c"}
	for i, w := range want {
		if items[i] != w {
			t.Fatalf("lrange[%d]: want %q, got %q", i, w, items[i])
		}
	}

	if err := s.ListSet("l", 1, "A"); err != nil {
		t.Fatalf("lset: %v", err)
	}
	if items, _ := s.GetListRange("l", 1, 1); items[0] != "A" {
		t.Fatalf("lset result: got %q", items[0])
	}

	if n, err := s.ListRemove("l", "b", 0); err != nil || n != 1 {
		t.Fatalf("lrem: got %d, %v", n, err)
	}

	if popped, err := s.ListPop("l", "left"); err != nil || popped != "head" {
		t.Fatalf("lpop: got %q, %v", popped, err)
	}
	if popped, err := s.ListPop("l", "right"); err != nil || popped != "c" {
		t.Fatalf("rpop: got %q, %v", popped, err)
	}
}

func TestRedisSetOps(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	if n, err := s.SetAdd("s", []string{"a", "b", "c"}); err != nil || n != 3 {
		t.Fatalf("sadd: got %d, %v", n, err)
	}
	members, err := s.GetSetAll("s")
	if err != nil || len(members) != 3 {
		t.Fatalf("smembers: got %v, %v", members, err)
	}
	if n, err := s.SetRemove("s", []string{"a", "ghost"}); err != nil || n != 1 {
		t.Fatalf("srem: got %d, %v", n, err)
	}
}

func TestRedisZSetOps(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	members := []ScoredMember{
		{Score: 92.3, Member: "bob"},
		{Score: 98.5, Member: "alice"},
		{Score: 75.2, Member: "dave"},
	}
	if n, err := s.ZSetAdd("z", members); err != nil || n != 3 {
		t.Fatalf("zadd: got %d, %v", n, err)
	}

	got, err := s.GetSortedSetRange("z", "-inf", "+inf")
	if err != nil {
		t.Fatalf("zrangebyscore: %v", err)
	}
	wantMembers := []string{"dave", "bob", "alice"}
	wantScores := []float64{75.2, 92.3, 98.5}
	for i, g := range got {
		if g.Member != wantMembers[i] || g.Score != wantScores[i] {
			t.Fatalf("zrangebyscore[%d]: want {%v %s}, got {%v %s}", i, wantScores[i], wantMembers[i], g.Score, g.Member)
		}
	}

	if got, err = s.GetSortedSetRange("z", "90", "99"); err != nil || len(got) != 2 {
		t.Fatalf("zrangebyscore 90..99: got %+v, %v", got, err)
	}

	if n, err := s.ZSetRemove("z", []string{"dave", "ghost"}); err != nil || n != 1 {
		t.Fatalf("zrem: got %d, %v", n, err)
	}
}

func TestRedisScanKeys(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	mr.Set("user:1", "a")
	mr.Set("user:2", "b")
	mr.Set("user:3", "c")
	mr.SetTTL("user:2", 120*time.Second)
	mr.Set("other:1", "x")

	result, err := s.ScanKeys("user:*", 0, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Keys) != 3 {
		t.Fatalf("scan keys: got %d entries %+v", len(result.Keys), result.Keys)
	}
	for _, k := range result.Keys {
		if k.Type != "string" {
			t.Fatalf("key %s: type %q, want string", k.Name, k.Type)
		}
		wantTTL := int64(-1)
		if k.Name == "user:2" {
			wantTTL = 120
		}
		if k.TTL != wantTTL {
			t.Fatalf("key %s: ttl %d, want %d", k.Name, k.TTL, wantTTL)
		}
	}

	// A deleted key mid-page reports TTL -2 and is skipped.
	mr.Del("user:3")
	result, err = s.ScanKeys("user:*", 0, 10)
	if err != nil {
		t.Fatalf("scan 2: %v", err)
	}
	if len(result.Keys) != 2 || result.ScanCount != 2 {
		t.Fatalf("scan 2: got %+v count %d", result.Keys, result.ScanCount)
	}
}

func TestRedisKeyInfoAndTTL(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	mr.Set("k", "v")
	mr.SetTTL("k", 60*time.Second)

	info, err := s.GetKeyInfo("k")
	if err != nil {
		t.Fatalf("keyinfo: %v", err)
	}
	if info.Type != "string" || info.TTL != 60 {
		t.Fatalf("keyinfo: got %+v", info)
	}

	ttl, err := s.GetKeyTTL("k")
	if err != nil || ttl != 60 {
		t.Fatalf("ttl: got %d, %v", ttl, err)
	}

	if err := s.SetKeyTTL("k", -1); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if ttl, _ = s.GetKeyTTL("k"); ttl != -1 {
		t.Fatalf("ttl after persist: got %d", ttl)
	}

	if err := s.SetKeyTTL("k", 30); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if ttl, _ = s.GetKeyTTL("k"); ttl != 30 {
		t.Fatalf("ttl after expire: got %d", ttl)
	}

	if _, err := s.GetKeyInfo("ghost"); err == nil || !strings.Contains(err.Error(), "key not found") {
		t.Fatalf("ghost keyinfo: got %v", err)
	}
}

func TestRedisSwitchDB(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	if s.DBIndex() != 0 {
		t.Fatalf("initial db index: %d", s.DBIndex())
	}
	if err := s.SwitchDB(1); err != nil {
		t.Fatalf("switchdb: %v", err)
	}
	if s.DBIndex() != 1 {
		t.Fatalf("db index after switch: %d", s.DBIndex())
	}

	// Writes after the switch must land in db 1, and new pooled connections
	// must be re-selected onto the same db.
	if err := s.SetString("db1key", "v"); err != nil {
		t.Fatalf("set after switch: %v", err)
	}
	if mr.DB(1).Exists("db1key") != true {
		t.Fatal("key not found in db 1")
	}
	if v, err := s.GetString("db1key"); err != nil || v != "v" {
		t.Fatalf("get after switch: got %q, %v", v, err)
	}

	if err := s.SwitchDB(-1); err == nil {
		t.Fatal("negative db index accepted")
	}
}

func TestRedisDBSizeAndKeyspace(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	mr.Set("a", "1")
	mr.Set("b", "2")

	if n, err := s.DBSize(); err != nil || n != 2 {
		t.Fatalf("dbsize: got %d, %v", n, err)
	}

	// miniredis does not implement `INFO keyspace`, so the line parser is
	// verified directly against real-server payload shapes.
	ks := parseKeyspaceInfo("# Keyspace\r\ndb0:keys=2,expires=0,avg_ttl=0\r\ndb1:keys=5,expires=1,avg_ttl=100\r\n")
	if ks[0] != 2 || ks[1] != 5 || len(ks) != 2 {
		t.Fatalf("keyspace parse: got %v", ks)
	}
	if ks := parseKeyspaceInfo("# Keyspace\r\n"); len(ks) != 0 {
		t.Fatalf("keyspace parse empty: got %v", ks)
	}

	// End-to-end still works against a server that supports the section;
	// skipped when the backend lacks it (miniredis).
	if ks, err := s.KeyspaceInfo(); err == nil && ks[0] != 2 {
		t.Fatalf("keyspace live db0: got %d", ks[0])
	}
}

func TestRedisDBCount(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")

	// miniredis has no CONFIG GET databases, so DBCount falls back to
	// INFO keyspace (db0 only -> 1) before the default of 16.
	n, err := s.DBCount()
	if err != nil || n < 1 {
		t.Fatalf("dbcount: got %d, %v", n, err)
	}
}

func TestRedisNotConnected(t *testing.T) {
	s := NewRedisSession("test-disconnected")
	if s.IsConnected() {
		t.Fatal("fresh session marked connected")
	}
	if err := s.Ping(); err == nil || err.Error() != "not connected" {
		t.Fatalf("ping: got %v", err)
	}
	if _, err := s.GetString("k"); err == nil {
		t.Fatal("get on disconnected session should fail")
	}
}

func TestRedisWriteResizeNoop(t *testing.T) {
	s := NewRedisSession("test-noop")
	if err := s.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := s.Resize(80, 24); err != nil {
		t.Fatalf("resize: %v", err)
	}
}

// fakeSentinel speaks just enough RESP to answer the one query
// FZambia/sentinel uses for master resolution
// (SENTINEL get-master-addr-by-name): every query gets a bulk-string pair
// pointing at masterAddr.
func fakeSentinel(t *testing.T, masterAddr string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeSentinel(conn, masterAddr)
		}
	}()
	return ln.Addr().String()
}

func serveFakeSentinel(conn net.Conn, masterAddr string) {
	defer conn.Close()
	host, port, _ := net.SplitHostPort(masterAddr)
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if !strings.HasPrefix(line, "*") {
			continue
		}
		n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
		cmd := make([]string, 0, n)
		for i := 0; i < n*2; i++ {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if i%2 == 1 {
				cmd = append(cmd, strings.TrimSpace(l))
			}
		}
		if len(cmd) > 1 && strings.EqualFold(cmd[0], "SENTINEL") && strings.EqualFold(cmd[1], "get-master-addr-by-name") {
			fmt.Fprintf(conn, "*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(host), host, len(port), port)
		} else {
			conn.Write([]byte("*0\r\n"))
		}
	}
}

func TestRedisSentinelConnect(t *testing.T) {
	mr := newTestRedis(t, "secret")
	sentinelAddr := fakeSentinel(t, mr.Addr())

	s := NewRedisSession("test-sentinel")
	err := s.Connect(ConnectionConfig{
		RedisMode:       "sentinel",
		RedisSentinels:  sentinelAddr,
		RedisMasterName: "mymaster",
		Password:        "secret",
	})
	if err != nil {
		t.Fatalf("sentinel connect: %v", err)
	}
	defer s.Disconnect()

	// Master auth must have been applied on the resolved address.
	if _, err := s.GetString("nope"); err == nil || err.Error() != "redis: nil" {
		t.Fatalf("get via sentinel: got %v", err)
	}
	if err := s.SetString("k", "v"); err != nil {
		t.Fatalf("set via sentinel: %v", err)
	}
	if mr.Exists("k") != true {
		t.Fatal("key did not reach the master behind the sentinel")
	}
}

func TestRedisSentinelBadConfig(t *testing.T) {
	s := NewRedisSession("test-sentinel-bad")
	err := s.Connect(ConnectionConfig{RedisMode: "sentinel", RedisSentinels: "  ", RedisMasterName: "m"})
	if err == nil || !strings.Contains(err.Error(), "no sentinel addresses") {
		t.Fatalf("empty sentinel list: got %v", err)
	}
	err = s.Connect(ConnectionConfig{RedisMode: "sentinel", RedisSentinels: "127.0.0.1:26379", RedisMasterName: ""})
	if err == nil || !strings.Contains(err.Error(), "master name required") {
		t.Fatalf("empty master name: got %v", err)
	}
}

// Guard against accidental blocking behavior: a full round trip of the
// heaviest path (scan page + TYPE/TTL pipeline) must stay well under the
// per-op timeouts.
func TestRedisScanKeysLatency(t *testing.T) {
	mr := newTestRedis(t, "")
	s := newTestSession(t, mr, "")
	for i := 0; i < 50; i++ {
		mr.Set(fmt.Sprintf("k%d", i), "v")
	}
	start := time.Now()
	if _, err := s.ScanKeys("*", 0, 100); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("scan too slow: %v", elapsed)
	}
}
