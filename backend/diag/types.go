package diag

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Entry struct {
	Ts         string         `json:"ts"`
	Level      Level          `json:"level"`
	Tag        string         `json:"tag"`
	Msg        string         `json:"msg"`
	Fields     map[string]any `json:"fields,omitempty"`
	Caller     *Caller        `json:"caller,omitempty"`
	Goroutine  string         `json:"goroutine,omitempty"`
	DedupCount int            `json:"dedup_count"`
	Dropped    int            `json:"dropped"`
}

type Caller struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type Summary struct {
	Levels       map[Level]int64 `json:"levels"`
	Ops          []OpStat        `json:"ops"`
	DroppedTotal int64           `json:"droppedTotal"`
	DedupTotal   int64           `json:"dedupTotal"`
}

type OpStat struct {
	Name    string  `json:"name"`
	Count   int64   `json:"count"`
	P50Ms   float64 `json:"p50Ms"`
	P95Ms   float64 `json:"p95Ms"`
	P99Ms   float64 `json:"p99Ms"`
	LastErr string  `json:"lastErr,omitempty"`
}
